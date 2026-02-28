// internal/lsp/server.go
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// AnalyzeFunc is the function signature for analyzing a file
type AnalyzeFunc func(ctx context.Context, path, content string) ([]sarif.Result, error)

// resultsCacheEntry holds cached SARIF results for a document
type resultsCacheEntry struct {
	results     []sarif.Result
	diagnostics []Diagnostic
}

// ServerConfig holds configuration for the LSP server
type ServerConfig struct {
	DebounceDuration time.Duration
	ParallelFiles    int
	WatchPatterns    []string
	IgnorePatterns   []string
}

// DefaultServerConfig returns sensible defaults
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		DebounceDuration: 300 * time.Millisecond,
		ParallelFiles:    3,
		WatchPatterns: []string{
			"**/*.go", "**/*.py", "**/*.ts", "**/*.tsx", "**/*.js", "**/*.jsx",
		},
		IgnorePatterns: []string{
			"**/node_modules/**", "**/.git/**", "**/vendor/**", "**/.gavel/**",
		},
	}
}

// ServerConfigFromLSPConfig converts config.LSPConfig to ServerConfig
func ServerConfigFromLSPConfig(lspCfg config.LSPConfig) ServerConfig {
	cfg := DefaultServerConfig()

	if lspCfg.Watcher.DebounceDuration != "" {
		if d, err := ParseDuration(lspCfg.Watcher.DebounceDuration); err == nil {
			cfg.DebounceDuration = d
		}
	}
	if lspCfg.Analysis.ParallelFiles > 0 {
		cfg.ParallelFiles = lspCfg.Analysis.ParallelFiles
	}
	if len(lspCfg.Watcher.WatchPatterns) > 0 {
		cfg.WatchPatterns = lspCfg.Watcher.WatchPatterns
	}
	if len(lspCfg.Watcher.IgnorePatterns) > 0 {
		cfg.IgnorePatterns = lspCfg.Watcher.IgnorePatterns
	}

	return cfg
}

// Server implements an LSP server
type Server struct {
	reader  *bufio.Reader
	writer  *bufio.Writer
	analyze AnalyzeFunc

	// Document tracking
	documents map[string]string // URI -> content
	docMu     sync.RWMutex

	// Results cache for code actions
	resultsCache map[string]resultsCacheEntry
	resultsMu    sync.RWMutex

	// Components
	watcher      *DebouncedWatcher
	cacheManager cache.CacheManager
	progress     *ProgressReporter
	commands     *CommandHandler
	config       ServerConfig

	// State
	rootURI     string
	initialized bool
}

// NewServer creates a new LSP server with default configuration
func NewServer(reader *bufio.Reader, writer *bufio.Writer, analyze AnalyzeFunc) *Server {
	return NewServerWithConfig(reader, writer, analyze, DefaultServerConfig())
}

// NewServerWithConfig creates a new LSP server with custom configuration
func NewServerWithConfig(reader *bufio.Reader, writer *bufio.Writer, analyze AnalyzeFunc, cfg ServerConfig) *Server {
	s := &Server{
		reader:       reader,
		writer:       writer,
		analyze:      analyze,
		documents:    make(map[string]string),
		resultsCache: make(map[string]resultsCacheEntry),
		config:       cfg,
	}

	// Initialize progress reporter
	s.progress = NewProgressReporter(s.sendMessage)

	// Initialize command handler
	s.commands = NewCommandHandler(s)

	// Initialize debounced watcher with configuration
	watcherConfig := WatcherConfig{
		DebounceDuration: cfg.DebounceDuration,
		ParallelFiles:    cfg.ParallelFiles,
		WatchPatterns:    cfg.WatchPatterns,
		IgnorePatterns:   cfg.IgnorePatterns,
	}
	s.watcher = NewDebouncedWatcherWithConfig(watcherConfig, func(files []string) {
		for _, uri := range files {
			s.docMu.RLock()
			content, ok := s.documents[uri]
			s.docMu.RUnlock()

			if ok {
				path := uriToPath(uri)
				s.analyzeAndPublish(context.Background(), uri, path, content)
			}
		}
	})

	return s
}

// SetCacheManager sets the cache manager for the server
func (s *Server) SetCacheManager(c cache.CacheManager) {
	s.cacheManager = c
}

// jsonRPCMessage represents a JSON-RPC 2.0 message
type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   interface{}     `json:"error,omitempty"`
}

// Run starts the LSP server message loop
func (s *Server) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := s.handleMessage(ctx); err != nil {
				if err == io.EOF {
					return nil
				}
				slog.Error("error handling message", "err", err)
			}
		}
	}
}

// handleMessage reads and processes a single JSON-RPC message
func (s *Server) handleMessage(ctx context.Context) error {
	// Read Content-Length header
	header, err := s.reader.ReadString('\n')
	if err != nil {
		return err
	}

	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "Content-Length:") {
		return fmt.Errorf("invalid header: %s", header)
	}

	lengthStr := strings.TrimSpace(strings.TrimPrefix(header, "Content-Length:"))
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return fmt.Errorf("invalid content length: %s", lengthStr)
	}

	// Read empty line
	if _, err := s.reader.ReadString('\n'); err != nil {
		return err
	}

	// Read message body
	buf := make([]byte, length)
	if _, err := io.ReadFull(s.reader, buf); err != nil {
		return err
	}

	// Parse JSON-RPC message
	var msg jsonRPCMessage
	if err := json.Unmarshal(buf, &msg); err != nil {
		return fmt.Errorf("failed to parse JSON-RPC message: %w", err)
	}

	// Handle request
	switch msg.Method {
	case MethodInitialize:
		return s.handleInitialize(msg.ID, msg.Params)
	case MethodInitialized:
		s.initialized = true
		return nil
	case MethodTextDocumentDidOpen:
		return s.handleDidOpen(ctx, msg.Params)
	case MethodTextDocumentDidSave:
		return s.handleDidSave(ctx, msg.Params)
	case MethodTextDocumentDidClose:
		return s.handleDidClose(msg.Params)
	case MethodTextDocumentCodeAction:
		return s.handleCodeAction(ctx, msg.ID, msg.Params)
	case MethodWorkspaceExecuteCommand:
		return s.handleExecuteCommand(ctx, msg.ID, msg.Params)
	case MethodWorkspaceDidChangeConfig:
		return s.handleDidChangeConfiguration(msg.Params)
	case MethodShutdown:
		return s.handleShutdown(msg.ID)
	case MethodExit:
		return io.EOF
	default:
		slog.Warn("unhandled LSP method", "method", msg.Method)
		return nil
	}
}

// handleInitialize processes the initialize request
func (s *Server) handleInitialize(id interface{}, params json.RawMessage) error {
	var initParams InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return err
	}

	s.rootURI = initParams.RootURI

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full sync
				Save:      true,
			},
			CodeActionProvider: true,
			ExecuteCommandProvider: &ExecuteCommandOptions{
				Commands: []string{
					CommandAnalyzeFile,
					CommandAnalyzeWorkspace,
					CommandClearCache,
				},
			},
		},
		ServerInfo: &ServerInfo{
			Name:    "gavel-lsp",
			Version: "0.2.0",
		},
	}

	return s.sendResponse(id, result, nil)
}

// handleDidOpen processes textDocument/didOpen notification
func (s *Server) handleDidOpen(ctx context.Context, params json.RawMessage) error {
	var didOpenParams DidOpenTextDocumentParams
	if err := json.Unmarshal(params, &didOpenParams); err != nil {
		return err
	}

	uri := didOpenParams.TextDocument.URI
	content := didOpenParams.TextDocument.Text

	// Check if we should watch this file
	if !s.shouldAnalyze(uri) {
		return nil
	}

	// Store document content
	s.docMu.Lock()
	s.documents[uri] = content
	s.docMu.Unlock()

	// Trigger analysis via watcher (debounced)
	s.watcher.FileChanged(uri)

	return nil
}

// handleDidSave processes textDocument/didSave notification
func (s *Server) handleDidSave(ctx context.Context, params json.RawMessage) error {
	var didSaveParams DidSaveTextDocumentParams
	if err := json.Unmarshal(params, &didSaveParams); err != nil {
		return err
	}

	uri := didSaveParams.TextDocument.URI

	// Check if we should watch this file
	if !s.shouldAnalyze(uri) {
		return nil
	}

	// If text is provided in the save notification, update it
	if didSaveParams.Text != nil {
		s.docMu.Lock()
		s.documents[uri] = *didSaveParams.Text
		s.docMu.Unlock()
	}

	// Trigger analysis via watcher (debounced)
	s.watcher.FileChanged(uri)

	return nil
}

// handleDidClose processes textDocument/didClose notification
func (s *Server) handleDidClose(params json.RawMessage) error {
	var didCloseParams DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &didCloseParams); err != nil {
		return err
	}

	uri := didCloseParams.TextDocument.URI

	// Remove document from tracking
	s.docMu.Lock()
	delete(s.documents, uri)
	s.docMu.Unlock()

	// Remove from results cache
	s.resultsMu.Lock()
	delete(s.resultsCache, uri)
	s.resultsMu.Unlock()

	return nil
}

// handleShutdown processes the shutdown request
func (s *Server) handleShutdown(id interface{}) error {
	// Stop the watcher
	if s.watcher != nil {
		s.watcher.Stop()
	}
	return s.sendResponse(id, nil, nil)
}

// handleCodeAction processes textDocument/codeAction requests
func (s *Server) handleCodeAction(ctx context.Context, id interface{}, params json.RawMessage) error {
	var caParams CodeActionParams
	if err := json.Unmarshal(params, &caParams); err != nil {
		return s.sendResponse(id, nil, map[string]interface{}{
			"code":    -32602,
			"message": fmt.Sprintf("invalid params: %v", err),
		})
	}

	uri := caParams.TextDocument.URI

	// Get cached results for this document
	s.resultsMu.RLock()
	entry, ok := s.resultsCache[uri]
	s.resultsMu.RUnlock()

	if !ok {
		// No results cached - return empty actions
		return s.sendResponse(id, []CodeAction{}, nil)
	}

	// Filter diagnostics that overlap with the requested range
	relevantDiags := FilterDiagnosticsForRange(entry.diagnostics, caParams.Range)
	if len(relevantDiags) == 0 {
		return s.sendResponse(id, []CodeAction{}, nil)
	}

	// Generate code actions for relevant diagnostics
	actions := GetCodeActions(uri, relevantDiags, entry.results)

	return s.sendResponse(id, actions, nil)
}

// handleExecuteCommand processes workspace/executeCommand requests
func (s *Server) handleExecuteCommand(ctx context.Context, id interface{}, params json.RawMessage) error {
	var execParams ExecuteCommandParams
	if err := json.Unmarshal(params, &execParams); err != nil {
		return s.sendResponse(id, nil, map[string]interface{}{
			"code":    -32602,
			"message": fmt.Sprintf("invalid params: %v", err),
		})
	}

	result, err := s.commands.Execute(ctx, execParams)
	if err != nil {
		return s.sendResponse(id, nil, map[string]interface{}{
			"code":    -32603,
			"message": err.Error(),
		})
	}

	return s.sendResponse(id, result, nil)
}

// handleDidChangeConfiguration processes workspace/didChangeConfiguration notifications
func (s *Server) handleDidChangeConfiguration(params json.RawMessage) error {
	var configParams DidChangeConfigurationParams
	if err := json.Unmarshal(params, &configParams); err != nil {
		return err
	}

	// Try to extract gavel settings from the configuration
	settingsJSON, err := json.Marshal(configParams.Settings)
	if err != nil {
		return nil // Ignore invalid settings
	}

	// Try to unmarshal as a map with "gavel" key
	var settingsMap map[string]json.RawMessage
	if err := json.Unmarshal(settingsJSON, &settingsMap); err == nil {
		if gavelSettings, ok := settingsMap["gavel"]; ok {
			settingsJSON = gavelSettings
		}
	}

	var settings GavelSettings
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return nil // Ignore invalid settings
	}

	// Apply settings to watcher
	newConfig := WatcherConfig{}
	if settings.DebounceDuration != "" {
		if d, err := ParseDuration(settings.DebounceDuration); err == nil {
			newConfig.DebounceDuration = d
		}
	}
	if settings.ParallelFiles > 0 {
		newConfig.ParallelFiles = settings.ParallelFiles
	}
	if len(settings.WatchPatterns) > 0 {
		newConfig.WatchPatterns = settings.WatchPatterns
	}
	if len(settings.IgnorePatterns) > 0 {
		newConfig.IgnorePatterns = settings.IgnorePatterns
	}

	s.watcher.UpdateConfig(newConfig)

	return nil
}

// analyzeAndPublish runs analysis on a file and publishes diagnostics
func (s *Server) analyzeAndPublish(ctx context.Context, uri, path, content string) {
	// Run analysis
	results, err := s.analyze(ctx, path, content)
	if err != nil {
		slog.Error("analysis failed", "uri", uri, "err", err)
		return
	}

	// Convert to diagnostics
	diagnostics := SarifResultsToDiagnostics(results)

	// Cache results for code actions
	s.resultsMu.Lock()
	s.resultsCache[uri] = resultsCacheEntry{
		results:     results,
		diagnostics: diagnostics,
	}
	s.resultsMu.Unlock()

	// Publish diagnostics
	if err := s.publishDiagnostics(uri, diagnostics); err != nil {
		slog.Error("failed to publish diagnostics", "uri", uri, "err", err)
	}
}

// shouldAnalyze checks if a file should be analyzed based on watch/ignore patterns
func (s *Server) shouldAnalyze(uri string) bool {
	return ShouldWatchPath(uri, s.config.WatchPatterns, s.config.IgnorePatterns)
}

// publishDiagnostics sends a textDocument/publishDiagnostics notification
func (s *Server) publishDiagnostics(uri string, diagnostics []Diagnostic) error {
	params := PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diagnostics,
	}

	notification := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  MethodTextDocumentPublishDiagnostics,
		Params:  mustMarshal(params),
	}

	return s.sendMessage(notification)
}

// sendResponse sends a JSON-RPC response
func (s *Server) sendResponse(id interface{}, result interface{}, err interface{}) error {
	response := jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   err,
	}
	return s.sendMessage(response)
}

// sendMessage sends a JSON-RPC message with Content-Length header
func (s *Server) sendMessage(msg jsonRPCMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := s.writer.WriteString(header); err != nil {
		return err
	}
	if _, err := s.writer.Write(data); err != nil {
		return err
	}
	return s.writer.Flush()
}

// uriToPath converts a file:// URI to a filesystem path
func uriToPath(uri string) string {
	// Simple implementation: strip file:// prefix
	path := strings.TrimPrefix(uri, "file://")
	// On Windows, paths start with /C:/ which we should convert to C:/
	// For now, just remove the leading slash on Unix paths
	return path
}

// mustMarshal marshals v to JSON, panicking on error
func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return data
}
