// internal/lsp/server.go
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// AnalyzeFunc is the function signature for analyzing a file
type AnalyzeFunc func(ctx context.Context, path, content string) ([]sarif.Result, error)

// Server implements an LSP server
type Server struct {
	reader    *bufio.Reader
	writer    *bufio.Writer
	analyze   AnalyzeFunc
	cache     cache.CacheManager
	documents map[string]string // URI -> content
	watcher   *DebouncedWatcher
}

// NewServer creates a new LSP server
func NewServer(reader *bufio.Reader, writer *bufio.Writer, analyze AnalyzeFunc) *Server {
	s := &Server{
		reader:    reader,
		writer:    writer,
		analyze:   analyze,
		documents: make(map[string]string),
	}

	// Initialize debounced watcher for batch analysis
	s.watcher = NewDebouncedWatcher(300*time.Millisecond, func(files []string) {
		for _, uri := range files {
			if content, ok := s.documents[uri]; ok {
				path := uriToPath(uri)
				s.analyzeAndPublish(context.Background(), uri, path, content)
			}
		}
	})

	return s
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
				log.Printf("Error handling message: %v", err)
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
		// No response needed for notification
		return nil
	case MethodTextDocumentDidOpen:
		return s.handleDidOpen(ctx, msg.Params)
	case MethodTextDocumentDidSave:
		return s.handleDidSave(ctx, msg.Params)
	case MethodTextDocumentDidClose:
		return s.handleDidClose(msg.Params)
	case MethodShutdown:
		return s.handleShutdown(msg.ID)
	case MethodExit:
		return io.EOF
	default:
		log.Printf("Unhandled method: %s", msg.Method)
		return nil
	}
}

// handleInitialize processes the initialize request
func (s *Server) handleInitialize(id interface{}, params json.RawMessage) error {
	var initParams InitializeParams
	if err := json.Unmarshal(params, &initParams); err != nil {
		return err
	}

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full sync
				Save:      true,
			},
		},
		ServerInfo: &ServerInfo{
			Name:    "gavel-lsp",
			Version: "0.1.0",
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

	// Store document content
	s.documents[uri] = content

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

	// If text is provided in the save notification, update it
	if didSaveParams.Text != nil {
		s.documents[uri] = *didSaveParams.Text
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

	// Remove document from tracking
	delete(s.documents, didCloseParams.TextDocument.URI)

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

// analyzeAndPublish runs analysis on a file and publishes diagnostics
func (s *Server) analyzeAndPublish(ctx context.Context, uri, path, content string) {
	// Run analysis
	results, err := s.analyze(ctx, path, content)
	if err != nil {
		log.Printf("Analysis error for %s: %v", uri, err)
		return
	}

	// Convert to diagnostics
	diagnostics := SarifResultsToDiagnostics(results)

	// Publish diagnostics
	if err := s.publishDiagnostics(uri, diagnostics); err != nil {
		log.Printf("Failed to publish diagnostics for %s: %v", uri, err)
	}
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
