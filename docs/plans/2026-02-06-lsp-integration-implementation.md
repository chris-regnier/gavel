# LSP Integration Implementation Plan (Phase 2)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expose SARIF diagnostics via Language Server Protocol for in-editor AI-powered feedback.

**Architecture:** LSP server with file watcher, debouncing, content-addressable cache (local-first, remote optional), and SARIF-to-diagnostic mapping. Progressive updates publish diagnostics as analyses complete.

**Tech Stack:** go-lsp (jsonrpc2), fsnotify (file watching), existing SARIF/analyzer infrastructure

---

## Task 1: Cache Manager Interface and Local Implementation

Create the unified cache manager that will be shared between CLI and LSP modes.

**Files:**
- Create: `internal/cache/cache.go`
- Create: `internal/cache/local.go`
- Create: `internal/cache/local_test.go`

**Step 1: Write the failing test for CacheManager interface**

```go
// internal/cache/local_test.go
package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestLocalCacheGetMiss(t *testing.T) {
	dir := t.TempDir()
	cache := NewLocalCache(dir)

	key := CacheKey{FileHash: "abc123", Provider: "ollama", Model: "test"}
	_, err := cache.Get(context.Background(), key)
	if err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss, got %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cache/ -run TestLocalCacheGetMiss -v`
Expected: FAIL with "package not found" or similar

**Step 3: Write cache interface and error**

```go
// internal/cache/cache.go
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/chris-regnier/gavel/internal/sarif"
)

var ErrCacheMiss = errors.New("cache miss")

// CacheKey identifies a unique analysis result
type CacheKey struct {
	FileHash    string            `json:"file_hash"`
	FilePath    string            `json:"file_path"`
	Provider    string            `json:"provider"`
	Model       string            `json:"model"`
	BAMLVersion string            `json:"baml_version"`
	Policies    map[string]string `json:"policies"` // policy name -> instruction hash
}

// Hash computes deterministic cache key
func (k CacheKey) Hash() string {
	b, _ := json.Marshal(k)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// CacheEntry represents a cached analysis result
type CacheEntry struct {
	Key       CacheKey       `json:"key"`
	Results   []sarif.Result `json:"results"`
	Timestamp int64          `json:"timestamp"`
}

// CacheManager provides cached analysis results
type CacheManager interface {
	Get(ctx context.Context, key CacheKey) (*CacheEntry, error)
	Put(ctx context.Context, entry *CacheEntry) error
	Delete(ctx context.Context, key CacheKey) error
}
```

**Step 4: Write local cache implementation**

```go
// internal/cache/local.go
package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type LocalCache struct {
	dir string
}

func NewLocalCache(dir string) *LocalCache {
	return &LocalCache{dir: dir}
}

func (c *LocalCache) entryPath(key CacheKey) string {
	return filepath.Join(c.dir, key.Hash()+".json")
}

func (c *LocalCache) Get(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	path := c.entryPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *LocalCache) Put(ctx context.Context, entry *CacheEntry) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}

	entry.Timestamp = time.Now().Unix()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.entryPath(entry.Key), data, 0644)
}

func (c *LocalCache) Delete(ctx context.Context, key CacheKey) error {
	return os.Remove(c.entryPath(key))
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/cache/ -run TestLocalCacheGetMiss -v`
Expected: PASS

**Step 6: Add Put/Get round-trip test**

```go
// Add to internal/cache/local_test.go
func TestLocalCachePutGet(t *testing.T) {
	dir := t.TempDir()
	cache := NewLocalCache(dir)

	key := CacheKey{
		FileHash: "abc123",
		Provider: "ollama",
		Model:    "test",
		Policies: map[string]string{"policy1": "instruction-hash"},
	}
	entry := &CacheEntry{
		Key: key,
		Results: []sarif.Result{
			{RuleID: "test-rule", Level: "error", Message: sarif.Message{Text: "test"}},
		},
	}

	ctx := context.Background()
	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(got.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got.Results))
	}
	if got.Results[0].RuleID != "test-rule" {
		t.Errorf("expected rule ID test-rule, got %s", got.Results[0].RuleID)
	}
}
```

**Step 7: Run all cache tests**

Run: `go test ./internal/cache/ -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/cache/
git commit -m "$(cat <<'EOF'
feat(cache): add CacheManager interface and local implementation

Content-addressable caching for LSP mode. CacheKey hashes file content,
provider, model, and policy instructions for deterministic cache lookups.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: File Watcher with Debouncing

Implement file watching that batches changes and triggers analysis after a debounce period.

**Files:**
- Create: `internal/lsp/watcher.go`
- Create: `internal/lsp/watcher_test.go`

**Step 1: Write the failing test for debounced watcher**

```go
// internal/lsp/watcher_test.go
package lsp

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncedWatcher(t *testing.T) {
	var triggerCount atomic.Int32
	onTrigger := func(files []string) {
		triggerCount.Add(1)
	}

	w := NewDebouncedWatcher(50*time.Millisecond, onTrigger)

	// Simulate rapid file changes
	w.FileChanged("a.go")
	w.FileChanged("b.go")
	w.FileChanged("c.go")

	// Wait less than debounce period - should not trigger
	time.Sleep(30 * time.Millisecond)
	if triggerCount.Load() != 0 {
		t.Fatal("triggered too early")
	}

	// Wait for debounce to complete
	time.Sleep(50 * time.Millisecond)
	if triggerCount.Load() != 1 {
		t.Fatalf("expected 1 trigger, got %d", triggerCount.Load())
	}

	w.Stop()
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/lsp/ -run TestDebouncedWatcher -v`
Expected: FAIL with "package not found"

**Step 3: Implement debounced watcher**

```go
// internal/lsp/watcher.go
package lsp

import (
	"sync"
	"time"
)

// DebouncedWatcher batches file changes and triggers analysis after quiet period
type DebouncedWatcher struct {
	debounce  time.Duration
	onTrigger func(files []string)

	mu          sync.Mutex
	pending     map[string]struct{}
	timer       *time.Timer
	stopCh      chan struct{}
	stopped     bool
}

func NewDebouncedWatcher(debounce time.Duration, onTrigger func(files []string)) *DebouncedWatcher {
	return &DebouncedWatcher{
		debounce:  debounce,
		onTrigger: onTrigger,
		pending:   make(map[string]struct{}),
		stopCh:    make(chan struct{}),
	}
}

func (w *DebouncedWatcher) FileChanged(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stopped {
		return
	}

	w.pending[path] = struct{}{}

	// Reset timer on each change
	if w.timer != nil {
		w.timer.Stop()
	}

	w.timer = time.AfterFunc(w.debounce, w.flush)
}

func (w *DebouncedWatcher) flush() {
	w.mu.Lock()
	if w.stopped || len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}

	files := make([]string, 0, len(w.pending))
	for f := range w.pending {
		files = append(files, f)
	}
	w.pending = make(map[string]struct{})
	w.mu.Unlock()

	w.onTrigger(files)
}

func (w *DebouncedWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
	close(w.stopCh)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/lsp/ -run TestDebouncedWatcher -v`
Expected: PASS

**Step 5: Add test for multiple batches**

```go
// Add to internal/lsp/watcher_test.go
func TestDebouncedWatcherMultipleBatches(t *testing.T) {
	var batches [][]string
	var mu sync.Mutex
	onTrigger := func(files []string) {
		mu.Lock()
		batches = append(batches, files)
		mu.Unlock()
	}

	w := NewDebouncedWatcher(30*time.Millisecond, onTrigger)

	// First batch
	w.FileChanged("a.go")
	time.Sleep(50 * time.Millisecond)

	// Second batch
	w.FileChanged("b.go")
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	mu.Unlock()

	w.Stop()
}
```

**Step 6: Run all watcher tests**

Run: `go test ./internal/lsp/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/lsp/
git commit -m "$(cat <<'EOF'
feat(lsp): add debounced file watcher

Batches rapid file changes and triggers analysis callback after
configurable quiet period. Used by LSP server to avoid excessive
LLM calls during active editing.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: SARIF to LSP Diagnostic Mapper

Convert SARIF results to LSP diagnostic format.

**Files:**
- Create: `internal/lsp/diagnostic.go`
- Create: `internal/lsp/diagnostic_test.go`

**Step 1: Write the failing test**

```go
// internal/lsp/diagnostic_test.go
package lsp

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestSarifToDiagnostic(t *testing.T) {
	result := sarif.Result{
		RuleID: "shall-be-merged",
		Level:  "error",
		Message: sarif.Message{
			Text: "SQL injection risk",
		},
		Locations: []sarif.Location{
			{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
					Region:           sarif.Region{StartLine: 10, EndLine: 12},
				},
			},
		},
		Properties: map[string]interface{}{
			"gavel/confidence":     0.95,
			"gavel/explanation":    "Query uses string concatenation",
			"gavel/recommendation": "Use parameterized queries",
		},
	}

	diag := SarifToDiagnostic(result)

	if diag.Severity != DiagnosticSeverityError {
		t.Errorf("expected error severity, got %d", diag.Severity)
	}
	if diag.Range.Start.Line != 9 { // 0-indexed
		t.Errorf("expected start line 9, got %d", diag.Range.Start.Line)
	}
	if diag.Source != "gavel" {
		t.Errorf("expected source gavel, got %s", diag.Source)
	}
	if diag.Code != "shall-be-merged" {
		t.Errorf("expected code shall-be-merged, got %s", diag.Code)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/lsp/ -run TestSarifToDiagnostic -v`
Expected: FAIL with undefined function

**Step 3: Implement diagnostic types and mapper**

```go
// internal/lsp/diagnostic.go
package lsp

import (
	"github.com/chris-regnier/gavel/internal/sarif"
)

// LSP Diagnostic Severity (from LSP spec)
type DiagnosticSeverity int

const (
	DiagnosticSeverityError       DiagnosticSeverity = 1
	DiagnosticSeverityWarning     DiagnosticSeverity = 2
	DiagnosticSeverityInformation DiagnosticSeverity = 3
	DiagnosticSeverityHint        DiagnosticSeverity = 4
)

// Position represents a position in a text document (0-indexed)
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range represents a range in a text document
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Diagnostic represents an LSP diagnostic
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
	Data     interface{}        `json:"data,omitempty"`
}

// DiagnosticData holds gavel-specific diagnostic metadata
type DiagnosticData struct {
	Confidence     float64 `json:"confidence,omitempty"`
	Explanation    string  `json:"explanation,omitempty"`
	Recommendation string  `json:"recommendation,omitempty"`
}

// SarifToDiagnostic converts a SARIF result to an LSP diagnostic
func SarifToDiagnostic(result sarif.Result) Diagnostic {
	diag := Diagnostic{
		Severity: levelToSeverity(result.Level),
		Code:     result.RuleID,
		Source:   "gavel",
		Message:  result.Message.Text,
	}

	// Convert location (SARIF is 1-indexed, LSP is 0-indexed)
	if len(result.Locations) > 0 {
		loc := result.Locations[0].PhysicalLocation
		diag.Range = Range{
			Start: Position{Line: loc.Region.StartLine - 1, Character: 0},
			End:   Position{Line: loc.Region.EndLine - 1, Character: 0},
		}
	}

	// Extract gavel properties
	if result.Properties != nil {
		data := DiagnosticData{}
		if v, ok := result.Properties["gavel/confidence"].(float64); ok {
			data.Confidence = v
		}
		if v, ok := result.Properties["gavel/explanation"].(string); ok {
			data.Explanation = v
		}
		if v, ok := result.Properties["gavel/recommendation"].(string); ok {
			data.Recommendation = v
		}
		diag.Data = data
	}

	return diag
}

func levelToSeverity(level string) DiagnosticSeverity {
	switch level {
	case "error":
		return DiagnosticSeverityError
	case "warning":
		return DiagnosticSeverityWarning
	case "note":
		return DiagnosticSeverityInformation
	default:
		return DiagnosticSeverityHint
	}
}

// SarifResultsToDiagnostics converts multiple SARIF results
func SarifResultsToDiagnostics(results []sarif.Result) []Diagnostic {
	diags := make([]Diagnostic, len(results))
	for i, r := range results {
		diags[i] = SarifToDiagnostic(r)
	}
	return diags
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/lsp/ -run TestSarifToDiagnostic -v`
Expected: PASS

**Step 5: Add test for warning level**

```go
// Add to internal/lsp/diagnostic_test.go
func TestSarifToDiagnosticWarning(t *testing.T) {
	result := sarif.Result{
		RuleID: "code-quality",
		Level:  "warning",
		Message: sarif.Message{Text: "Unused variable"},
		Locations: []sarif.Location{
			{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
					Region:           sarif.Region{StartLine: 5, EndLine: 5},
				},
			},
		},
	}

	diag := SarifToDiagnostic(result)

	if diag.Severity != DiagnosticSeverityWarning {
		t.Errorf("expected warning severity, got %d", diag.Severity)
	}
}
```

**Step 6: Run all diagnostic tests**

Run: `go test ./internal/lsp/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/lsp/diagnostic.go internal/lsp/diagnostic_test.go
git commit -m "$(cat <<'EOF'
feat(lsp): add SARIF to LSP diagnostic mapper

Converts SARIF results to LSP diagnostic format with proper
1-indexed to 0-indexed line conversion. Preserves gavel metadata
(confidence, explanation, recommendation) in diagnostic data.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: LSP Protocol Types

Define core LSP protocol message types needed for diagnostic publishing.

**Files:**
- Create: `internal/lsp/protocol.go`
- Create: `internal/lsp/protocol_test.go`

**Step 1: Write test for message serialization**

```go
// internal/lsp/protocol_test.go
package lsp

import (
	"encoding/json"
	"testing"
)

func TestPublishDiagnosticsParams(t *testing.T) {
	params := PublishDiagnosticsParams{
		URI: "file:///test.go",
		Diagnostics: []Diagnostic{
			{
				Range:    Range{Start: Position{Line: 0}, End: Position{Line: 1}},
				Severity: DiagnosticSeverityError,
				Message:  "test",
				Source:   "gavel",
			},
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded PublishDiagnosticsParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.URI != "file:///test.go" {
		t.Errorf("expected URI file:///test.go, got %s", decoded.URI)
	}
	if len(decoded.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(decoded.Diagnostics))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/lsp/ -run TestPublishDiagnosticsParams -v`
Expected: FAIL with undefined type

**Step 3: Implement protocol types**

```go
// internal/lsp/protocol.go
package lsp

// LSP Method constants
const (
	MethodInitialize            = "initialize"
	MethodInitialized           = "initialized"
	MethodShutdown              = "shutdown"
	MethodExit                  = "exit"
	MethodTextDocumentDidOpen   = "textDocument/didOpen"
	MethodTextDocumentDidClose  = "textDocument/didClose"
	MethodTextDocumentDidSave   = "textDocument/didSave"
	MethodTextDocumentDidChange = "textDocument/didChange"
	MethodPublishDiagnostics    = "textDocument/publishDiagnostics"
)

// InitializeParams from client
type InitializeParams struct {
	ProcessID    int          `json:"processId,omitempty"`
	RootURI      string       `json:"rootUri,omitempty"`
	Capabilities interface{}  `json:"capabilities,omitempty"`
}

// InitializeResult to client
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ServerCapabilities advertises server features
type ServerCapabilities struct {
	TextDocumentSync           TextDocumentSyncOptions `json:"textDocumentSync,omitempty"`
	DiagnosticProvider         bool                    `json:"diagnosticProvider,omitempty"`
}

// TextDocumentSyncOptions specifies document sync behavior
type TextDocumentSyncOptions struct {
	OpenClose bool `json:"openClose,omitempty"`
	Change    int  `json:"change,omitempty"` // 0=None, 1=Full, 2=Incremental
	Save      bool `json:"save,omitempty"`
}

// TextDocumentIdentifier identifies a text document
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentItem represents an open text document
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// DidOpenTextDocumentParams for textDocument/didOpen
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidCloseTextDocumentParams for textDocument/didClose
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidSaveTextDocumentParams for textDocument/didSave
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         string                 `json:"text,omitempty"`
}

// PublishDiagnosticsParams for textDocument/publishDiagnostics
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/lsp/ -run TestPublishDiagnosticsParams -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/lsp/protocol.go internal/lsp/protocol_test.go
git commit -m "$(cat <<'EOF'
feat(lsp): add LSP protocol types

Core LSP message types for initialize, text document sync, and
diagnostic publishing. Follows LSP 3.17 specification.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: LSP Server Core

Implement the LSP server with JSON-RPC 2.0 message handling.

**Files:**
- Create: `internal/lsp/server.go`
- Create: `internal/lsp/server_test.go`

**Step 1: Write test for server initialization**

```go
// internal/lsp/server_test.go
package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestServerInitialize(t *testing.T) {
	// Create mock analyzer that returns no results
	mockAnalyzer := func(ctx context.Context, path string, content []byte) ([]Diagnostic, error) {
		return nil, nil
	}

	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	server := NewServer(in, out, mockAnalyzer, "/tmp/cache")

	// Send initialize request
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"processId":1234,"rootUri":"file:///test"}}`
	writeMessage(in, initReq)

	// Process one message
	go func() {
		server.handleMessage(context.Background())
	}()

	// Read response
	resp := readMessage(out)
	if !strings.Contains(resp, `"id":1`) {
		t.Errorf("expected response with id 1, got %s", resp)
	}
	if !strings.Contains(resp, `"capabilities"`) {
		t.Errorf("expected capabilities in response, got %s", resp)
	}
}

func writeMessage(w io.Writer, content string) {
	header := "Content-Length: " + string(rune(len(content))) + "\r\n\r\n"
	w.Write([]byte(header))
	w.Write([]byte(content))
}

func readMessage(r *bytes.Buffer) string {
	// Skip header
	for {
		line, _ := r.ReadString('\n')
		if line == "\r\n" || line == "\n" || line == "" {
			break
		}
	}
	return r.String()
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/lsp/ -run TestServerInitialize -v`
Expected: FAIL with undefined NewServer

**Step 3: Implement LSP server**

```go
// internal/lsp/server.go
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chris-regnier/gavel/internal/cache"
)

// AnalyzeFunc is called to analyze a file
type AnalyzeFunc func(ctx context.Context, path string, content []byte) ([]Diagnostic, error)

// Server implements the LSP protocol
type Server struct {
	reader   *bufio.Reader
	writer   io.Writer
	analyze  AnalyzeFunc
	cacheDir string

	mu          sync.Mutex
	initialized bool
	rootURI     string
	documents   map[string]string // URI -> content
	watcher     *DebouncedWatcher
	cache       *cache.LocalCache
}

// NewServer creates a new LSP server
func NewServer(in io.Reader, out io.Writer, analyze AnalyzeFunc, cacheDir string) *Server {
	s := &Server{
		reader:    bufio.NewReader(in),
		writer:    out,
		analyze:   analyze,
		cacheDir:  cacheDir,
		documents: make(map[string]string),
		cache:     cache.NewLocalCache(cacheDir),
	}

	// Setup debounced watcher (5 second debounce by default)
	s.watcher = NewDebouncedWatcher(5*time.Second, func(files []string) {
		for _, f := range files {
			s.analyzeAndPublish(context.Background(), f)
		}
	})

	return s
}

// Run starts the server main loop
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
				return err
			}
		}
	}
}

func (s *Server) handleMessage(ctx context.Context) error {
	// Read Content-Length header
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			contentLength, _ = strconv.Atoi(strings.TrimSpace(line[15:]))
		}
	}

	if contentLength == 0 {
		return nil
	}

	// Read content
	content := make([]byte, contentLength)
	if _, err := io.ReadFull(s.reader, content); err != nil {
		return err
	}

	// Parse JSON-RPC message
	var msg struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(content, &msg); err != nil {
		return err
	}

	// Handle method
	switch msg.Method {
	case MethodInitialize:
		return s.handleInitialize(ctx, msg.ID, msg.Params)
	case MethodInitialized:
		// No response needed
		return nil
	case MethodShutdown:
		return s.sendResponse(msg.ID, nil)
	case MethodExit:
		return io.EOF
	case MethodTextDocumentDidOpen:
		return s.handleDidOpen(ctx, msg.Params)
	case MethodTextDocumentDidSave:
		return s.handleDidSave(ctx, msg.Params)
	case MethodTextDocumentDidClose:
		return s.handleDidClose(ctx, msg.Params)
	}

	return nil
}

func (s *Server) handleInitialize(ctx context.Context, id interface{}, params json.RawMessage) error {
	var p InitializeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	s.mu.Lock()
	s.rootURI = p.RootURI
	s.initialized = true
	s.mu.Unlock()

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: TextDocumentSyncOptions{
				OpenClose: true,
				Change:    1, // Full sync
				Save:      true,
			},
			DiagnosticProvider: true,
		},
	}

	return s.sendResponse(id, result)
}

func (s *Server) handleDidOpen(ctx context.Context, params json.RawMessage) error {
	var p DidOpenTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	s.mu.Lock()
	s.documents[p.TextDocument.URI] = p.TextDocument.Text
	s.mu.Unlock()

	// Queue for analysis
	s.watcher.FileChanged(p.TextDocument.URI)
	return nil
}

func (s *Server) handleDidSave(ctx context.Context, params json.RawMessage) error {
	var p DidSaveTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	if p.Text != "" {
		s.mu.Lock()
		s.documents[p.TextDocument.URI] = p.Text
		s.mu.Unlock()
	}

	// Queue for analysis
	s.watcher.FileChanged(p.TextDocument.URI)
	return nil
}

func (s *Server) handleDidClose(ctx context.Context, params json.RawMessage) error {
	var p DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return err
	}

	s.mu.Lock()
	delete(s.documents, p.TextDocument.URI)
	s.mu.Unlock()

	// Clear diagnostics
	return s.publishDiagnostics(p.TextDocument.URI, nil)
}

func (s *Server) analyzeAndPublish(ctx context.Context, uri string) {
	s.mu.Lock()
	content, ok := s.documents[uri]
	s.mu.Unlock()

	if !ok {
		return
	}

	// Convert URI to file path
	path := uriToPath(uri)

	// Run analysis
	diags, err := s.analyze(ctx, path, []byte(content))
	if err != nil {
		// Log error but don't fail
		return
	}

	// Publish diagnostics (progressive update)
	s.publishDiagnostics(uri, diags)
}

func (s *Server) publishDiagnostics(uri string, diags []Diagnostic) error {
	if diags == nil {
		diags = []Diagnostic{}
	}

	return s.sendNotification(MethodPublishDiagnostics, PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

func (s *Server) sendResponse(id interface{}, result interface{}) error {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	return s.writeMessage(resp)
}

func (s *Server) sendNotification(method string, params interface{}) error {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	return s.writeMessage(msg)
}

func (s *Server) writeMessage(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.writer.Write([]byte(header)); err != nil {
		return err
	}
	_, err = s.writer.Write(data)
	return err
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	return filepath.FromSlash(u.Path)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/lsp/ -run TestServerInitialize -v`
Expected: PASS (or may need adjustment for message format)

**Step 5: Add test for document sync**

```go
// Add to internal/lsp/server_test.go
func TestServerDocumentSync(t *testing.T) {
	analyzed := make(chan string, 1)
	mockAnalyzer := func(ctx context.Context, path string, content []byte) ([]Diagnostic, error) {
		analyzed <- path
		return []Diagnostic{
			{Range: Range{}, Severity: DiagnosticSeverityError, Message: "test error"},
		}, nil
	}

	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	server := NewServer(in, out, mockAnalyzer, t.TempDir())

	// Simulate didOpen
	server.mu.Lock()
	server.initialized = true
	server.mu.Unlock()

	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:  "file:///test.go",
			Text: "package main",
		},
	}
	server.handleDidOpen(context.Background(), mustMarshal(params))

	// Document should be tracked
	server.mu.Lock()
	_, exists := server.documents["file:///test.go"]
	server.mu.Unlock()

	if !exists {
		t.Error("document not tracked after didOpen")
	}
}

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
```

**Step 6: Run all server tests**

Run: `go test ./internal/lsp/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/lsp/server.go internal/lsp/server_test.go
git commit -m "$(cat <<'EOF'
feat(lsp): add LSP server with JSON-RPC 2.0 handling

Implements core LSP protocol: initialize, textDocument/didOpen,
textDocument/didSave, textDocument/didClose, and publishDiagnostics.
Integrates with debounced watcher for batched analysis.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Analyzer Integration

Wire the LSP server to the existing BAML analyzer.

**Files:**
- Create: `internal/lsp/analyzer.go`
- Create: `internal/lsp/analyzer_test.go`

**Step 1: Write test for analyzer wrapper**

```go
// internal/lsp/analyzer_test.go
package lsp

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// mockBAMLClient implements analyzer.BAMLClient for testing
type mockBAMLClient struct {
	findings []analyzer.Finding
}

func (m *mockBAMLClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]analyzer.Finding, error) {
	return m.findings, nil
}

func TestAnalyzerWrapper(t *testing.T) {
	client := &mockBAMLClient{
		findings: []analyzer.Finding{
			{
				RuleID:         "test-rule",
				Confidence:     0.9,
				Message:        "test finding",
				StartLine:      10,
				EndLine:        12,
				Explanation:    "explanation",
				Recommendation: "fix it",
			},
		},
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{Name: "ollama"},
		Policies: map[string]config.Policy{
			"test-rule": {Enabled: true, Instruction: "test"},
		},
	}

	wrapper := NewAnalyzerWrapper(client, cfg)
	diags, err := wrapper.Analyze(context.Background(), "test.go", []byte("package main"))

	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	if diags[0].Message != "test finding" {
		t.Errorf("expected message 'test finding', got %s", diags[0].Message)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/lsp/ -run TestAnalyzerWrapper -v`
Expected: FAIL with undefined NewAnalyzerWrapper

**Step 3: Implement analyzer wrapper**

```go
// internal/lsp/analyzer.go
package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// AnalyzerWrapper adapts the BAML analyzer for LSP use
type AnalyzerWrapper struct {
	client analyzer.BAMLClient
	cfg    *config.Config
	cache  cache.CacheManager
}

// NewAnalyzerWrapper creates a new analyzer wrapper
func NewAnalyzerWrapper(client analyzer.BAMLClient, cfg *config.Config) *AnalyzerWrapper {
	return &AnalyzerWrapper{
		client: client,
		cfg:    cfg,
	}
}

// WithCache enables caching for the analyzer
func (a *AnalyzerWrapper) WithCache(c cache.CacheManager) *AnalyzerWrapper {
	a.cache = c
	return a
}

// Analyze runs analysis on a file and returns LSP diagnostics
func (a *AnalyzerWrapper) Analyze(ctx context.Context, path string, content []byte) ([]Diagnostic, error) {
	// Build cache key
	key := a.buildCacheKey(path, content)

	// Check cache first
	if a.cache != nil {
		entry, err := a.cache.Get(ctx, key)
		if err == nil {
			return SarifResultsToDiagnostics(entry.Results), nil
		}
	}

	// Format policies for BAML
	policies := a.formatPolicies()

	// Run analysis
	findings, err := a.client.AnalyzeCode(ctx, string(content), policies)
	if err != nil {
		return nil, fmt.Errorf("analysis failed: %w", err)
	}

	// Convert to SARIF results
	results := make([]sarif.Result, len(findings))
	for i, f := range findings {
		results[i] = findingToResult(f, path)
	}

	// Cache results
	if a.cache != nil {
		entry := &cache.CacheEntry{Key: key, Results: results}
		a.cache.Put(ctx, entry)
	}

	return SarifResultsToDiagnostics(results), nil
}

func (a *AnalyzerWrapper) buildCacheKey(path string, content []byte) cache.CacheKey {
	// Hash file content
	h := sha256.Sum256(content)
	fileHash := hex.EncodeToString(h[:])

	// Hash policy instructions
	policies := make(map[string]string)
	for name, p := range a.cfg.Policies {
		if p.Enabled {
			ph := sha256.Sum256([]byte(p.Instruction))
			policies[name] = hex.EncodeToString(ph[:])
		}
	}

	// Get model info
	model := ""
	switch a.cfg.Provider.Name {
	case "ollama":
		model = a.cfg.Provider.Ollama.Model
	case "openrouter":
		model = a.cfg.Provider.OpenRouter.Model
	case "anthropic":
		model = a.cfg.Provider.Anthropic.Model
	case "bedrock":
		model = a.cfg.Provider.Bedrock.Model
	case "openai":
		model = a.cfg.Provider.OpenAI.Model
	}

	return cache.CacheKey{
		FileHash: fileHash,
		FilePath: path,
		Provider: a.cfg.Provider.Name,
		Model:    model,
		Policies: policies,
	}
}

func (a *AnalyzerWrapper) formatPolicies() string {
	var lines []string
	for name, p := range a.cfg.Policies {
		if p.Enabled {
			lines = append(lines, fmt.Sprintf("- %s: %s", name, p.Instruction))
		}
	}
	return strings.Join(lines, "\n")
}

func findingToResult(f analyzer.Finding, path string) sarif.Result {
	level := "warning"
	if f.Confidence > 0.8 {
		level = "error"
	}

	return sarif.Result{
		RuleID:  f.RuleID,
		Level:   level,
		Message: sarif.Message{Text: f.Message},
		Locations: []sarif.Location{
			{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{URI: path},
					Region:           sarif.Region{StartLine: f.StartLine, EndLine: f.EndLine},
				},
			},
		},
		Properties: map[string]interface{}{
			"gavel/confidence":     f.Confidence,
			"gavel/explanation":    f.Explanation,
			"gavel/recommendation": f.Recommendation,
		},
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/lsp/ -run TestAnalyzerWrapper -v`
Expected: PASS

**Step 5: Add test with caching**

```go
// Add to internal/lsp/analyzer_test.go
func TestAnalyzerWrapperWithCache(t *testing.T) {
	callCount := 0
	client := &mockBAMLClient{
		findings: []analyzer.Finding{
			{RuleID: "test", Message: "cached"},
		},
	}

	// Wrap client to track calls
	originalAnalyze := client.AnalyzeCode
	client.AnalyzeCode = func(ctx context.Context, code string, policies string) ([]analyzer.Finding, error) {
		callCount++
		return originalAnalyze(ctx, code, policies)
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{Name: "ollama"},
		Policies: map[string]config.Policy{"test": {Enabled: true}},
	}

	localCache := cache.NewLocalCache(t.TempDir())
	wrapper := NewAnalyzerWrapper(client, cfg).WithCache(localCache)

	ctx := context.Background()
	content := []byte("package main")

	// First call - cache miss
	_, err := wrapper.Analyze(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("first analyze failed: %v", err)
	}

	// Reset call count
	callCount = 0

	// Second call - cache hit
	diags, err := wrapper.Analyze(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("second analyze failed: %v", err)
	}

	if callCount != 0 {
		t.Errorf("expected 0 BAML calls (cache hit), got %d", callCount)
	}

	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic from cache, got %d", len(diags))
	}
}
```

Note: The above test has a bug (reassigning `client.AnalyzeCode` won't work on the mock). Here's a corrected version:

```go
// Add to internal/lsp/analyzer_test.go
func TestAnalyzerWrapperWithCache(t *testing.T) {
	callCount := 0
	client := &countingMockClient{
		findings: []analyzer.Finding{{RuleID: "test", Message: "cached"}},
		count:    &callCount,
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{Name: "ollama"},
		Policies: map[string]config.Policy{"test": {Enabled: true}},
	}

	localCache := cache.NewLocalCache(t.TempDir())
	wrapper := NewAnalyzerWrapper(client, cfg).WithCache(localCache)

	ctx := context.Background()
	content := []byte("package main")

	// First call - cache miss
	_, err := wrapper.Analyze(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("first analyze failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call, got %d", callCount)
	}

	// Second call - cache hit
	diags, err := wrapper.Analyze(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("second analyze failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected still 1 call (cache hit), got %d", callCount)
	}

	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic from cache, got %d", len(diags))
	}
}

type countingMockClient struct {
	findings []analyzer.Finding
	count    *int
}

func (m *countingMockClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]analyzer.Finding, error) {
	*m.count++
	return m.findings, nil
}
```

**Step 6: Run all analyzer tests**

Run: `go test ./internal/lsp/ -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/lsp/analyzer.go internal/lsp/analyzer_test.go
git commit -m "$(cat <<'EOF'
feat(lsp): add analyzer wrapper with caching integration

Wraps BAML analyzer for LSP use. Converts findings to SARIF results
then to LSP diagnostics. Integrates with CacheManager for content-
addressable result caching.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: CLI Command for LSP Mode

Add the `gavel lsp` command to start the LSP server.

**Files:**
- Create: `cmd/gavel/lsp.go`
- Modify: `cmd/gavel/main.go`

**Step 1: Create lsp command file**

```go
// cmd/gavel/lsp.go
package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/lsp"
	"github.com/spf13/cobra"
)

func newLSPCmd() *cobra.Command {
	var (
		machineCfg string
		projectCfg string
		cacheDir   string
	)

	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Start the Language Server Protocol server",
		Long: `Start Gavel as an LSP server for in-editor AI-powered code analysis.

The server communicates via stdin/stdout using JSON-RPC 2.0 as per the LSP specification.

Example editor configuration (neovim):
  vim.lsp.start({
    name = 'gavel',
    cmd = { './gavel', 'lsp' },
    root_dir = vim.fn.getcwd(),
  })`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLSP(machineCfg, projectCfg, cacheDir)
		},
	}

	homeDir, _ := os.UserHomeDir()
	defaultMachineCfg := filepath.Join(homeDir, ".config", "gavel", "policies.yaml")

	cmd.Flags().StringVar(&machineCfg, "machine-config", defaultMachineCfg, "Path to machine-level config")
	cmd.Flags().StringVar(&projectCfg, "project-config", ".gavel/policies.yaml", "Path to project-level config")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", ".gavel/cache/local", "Path to local cache directory")

	return cmd
}

func runLSP(machineCfg, projectCfg, cacheDir string) error {
	// Load config
	cfg, err := config.LoadTiered(machineCfg, projectCfg)
	if err != nil {
		return err
	}

	// Create BAML client based on provider
	client, err := analyzer.NewBAMLLiveClient(cfg)
	if err != nil {
		return err
	}

	// Create cache
	localCache := cache.NewLocalCache(cacheDir)

	// Create analyzer wrapper
	wrapper := lsp.NewAnalyzerWrapper(client, cfg).WithCache(localCache)

	// Create and run server
	server := lsp.NewServer(
		os.Stdin,
		os.Stdout,
		wrapper.Analyze,
		cacheDir,
	)

	return server.Run(context.Background())
}
```

**Step 2: Add lsp command to main.go**

Find the root command setup in `cmd/gavel/main.go` and add:

```go
// Add to the init() or main() where commands are registered
rootCmd.AddCommand(newLSPCmd())
```

**Step 3: Verify compilation**

Run: `go build ./cmd/gavel`
Expected: Build succeeds

**Step 4: Test command help**

Run: `./gavel lsp --help`
Expected: Shows LSP command help text

**Step 5: Commit**

```bash
git add cmd/gavel/lsp.go cmd/gavel/main.go
git commit -m "$(cat <<'EOF'
feat(cli): add gavel lsp command

Starts Gavel as an LSP server for in-editor AI-powered code analysis.
Communicates via stdin/stdout using JSON-RPC 2.0. Integrates with
tiered config loading and local caching.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: LSP Configuration Support

Add LSP-specific configuration options.

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/defaults.go`
- Create: `internal/config/lsp_test.go`

**Step 1: Write test for LSP config**

```go
// internal/config/lsp_test.go
package config

import (
	"testing"
	"time"
)

func TestLSPConfigDefaults(t *testing.T) {
	cfg := SystemDefaults()

	if cfg.LSP.Watcher.DebounceDuration != 5*time.Minute {
		t.Errorf("expected 5m debounce, got %v", cfg.LSP.Watcher.DebounceDuration)
	}

	if cfg.LSP.Analysis.ParallelFiles != 3 {
		t.Errorf("expected 3 parallel files, got %d", cfg.LSP.Analysis.ParallelFiles)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLSPConfigDefaults -v`
Expected: FAIL with "cfg.LSP undefined"

**Step 3: Add LSP config types to config.go**

```go
// Add to internal/config/config.go

// LSPConfig holds LSP server configuration
type LSPConfig struct {
	Watcher  WatcherConfig  `yaml:"watcher"`
	Analysis AnalysisConfig `yaml:"analysis"`
	Cache    CacheConfig    `yaml:"cache"`
}

// WatcherConfig configures file watching behavior
type WatcherConfig struct {
	DebounceDuration time.Duration `yaml:"debounce_duration"`
	WatchPatterns    []string      `yaml:"watch_patterns"`
	IgnorePatterns   []string      `yaml:"ignore_patterns"`
}

// AnalysisConfig configures analysis behavior
type AnalysisConfig struct {
	ParallelFiles int    `yaml:"parallel_files"`
	Priority      string `yaml:"priority"`
}

// CacheConfig configures caching behavior
type CacheConfig struct {
	TTL       time.Duration `yaml:"ttl"`
	MaxSizeMB int           `yaml:"max_size_mb"`
}

// Update Config struct to include LSP
type Config struct {
	Provider ProviderConfig    `yaml:"provider"`
	Policies map[string]Policy `yaml:"policies"`
	LSP      LSPConfig         `yaml:"lsp"`
}
```

**Step 4: Add defaults to defaults.go**

```go
// Update internal/config/defaults.go SystemDefaults()

func SystemDefaults() *Config {
	return &Config{
		// ... existing defaults ...
		LSP: LSPConfig{
			Watcher: WatcherConfig{
				DebounceDuration: 5 * time.Minute,
				WatchPatterns: []string{
					"**/*.go",
					"**/*.py",
					"**/*.ts",
					"**/*.tsx",
					"**/*.js",
					"**/*.jsx",
				},
				IgnorePatterns: []string{
					"**/node_modules/**",
					"**/.git/**",
					"**/vendor/**",
					"**/.gavel/**",
				},
			},
			Analysis: AnalysisConfig{
				ParallelFiles: 3,
				Priority:      "changed",
			},
			Cache: CacheConfig{
				TTL:       7 * 24 * time.Hour,
				MaxSizeMB: 500,
			},
		},
	}
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLSPConfigDefaults -v`
Expected: PASS

**Step 6: Update MergeConfigs for LSP settings**

```go
// Add to MergeConfigs in config.go, after policy merging

// Merge LSP config
if cfg.LSP.Watcher.DebounceDuration != 0 {
	result.LSP.Watcher.DebounceDuration = cfg.LSP.Watcher.DebounceDuration
}
if len(cfg.LSP.Watcher.WatchPatterns) > 0 {
	result.LSP.Watcher.WatchPatterns = cfg.LSP.Watcher.WatchPatterns
}
if len(cfg.LSP.Watcher.IgnorePatterns) > 0 {
	result.LSP.Watcher.IgnorePatterns = cfg.LSP.Watcher.IgnorePatterns
}
if cfg.LSP.Analysis.ParallelFiles != 0 {
	result.LSP.Analysis.ParallelFiles = cfg.LSP.Analysis.ParallelFiles
}
if cfg.LSP.Analysis.Priority != "" {
	result.LSP.Analysis.Priority = cfg.LSP.Analysis.Priority
}
if cfg.LSP.Cache.TTL != 0 {
	result.LSP.Cache.TTL = cfg.LSP.Cache.TTL
}
if cfg.LSP.Cache.MaxSizeMB != 0 {
	result.LSP.Cache.MaxSizeMB = cfg.LSP.Cache.MaxSizeMB
}
```

**Step 7: Run all config tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/config/
git commit -m "$(cat <<'EOF'
feat(config): add LSP configuration options

Adds watcher (debounce, patterns), analysis (parallelism, priority),
and cache (TTL, size limit) settings. Merges properly with tiered
config system.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Integration Test

End-to-end test of the LSP server with mock editor interaction.

**Files:**
- Create: `internal/lsp/integration_test.go`

**Step 1: Write integration test**

```go
// internal/lsp/integration_test.go
package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
)

func TestLSPIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Setup mock analyzer that returns findings
	client := &mockBAMLClient{
		findings: []analyzer.Finding{
			{
				RuleID:         "security-issue",
				Confidence:     0.95,
				Message:        "SQL injection vulnerability",
				StartLine:      5,
				EndLine:        5,
				Explanation:    "Query concatenates user input",
				Recommendation: "Use parameterized queries",
			},
		},
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{Name: "ollama"},
		Policies: map[string]config.Policy{
			"security-issue": {Enabled: true, Instruction: "Find SQL injection"},
		},
	}

	localCache := cache.NewLocalCache(t.TempDir())
	wrapper := NewAnalyzerWrapper(client, cfg).WithCache(localCache)

	// Create pipe for communication
	serverIn := &bytes.Buffer{}
	serverOut := &bytes.Buffer{}

	// Use shorter debounce for testing
	server := &Server{
		reader:    bufio.NewReader(serverIn),
		writer:    serverOut,
		analyze:   wrapper.Analyze,
		documents: make(map[string]string),
		cache:     localCache,
	}
	server.watcher = NewDebouncedWatcher(100*time.Millisecond, func(files []string) {
		for _, f := range files {
			server.analyzeAndPublish(context.Background(), f)
		}
	})

	// Initialize
	initReq := jsonRPCRequest(1, "initialize", InitializeParams{
		ProcessID: 1234,
		RootURI:   "file:///project",
	})
	serverIn.WriteString(lspMessage(initReq))
	server.handleMessage(context.Background())

	// Open document
	openReq := jsonRPCNotification("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///project/main.go",
			LanguageID: "go",
			Version:    1,
			Text:       "package main\n\nfunc main() {\n\tdb.Query(\"SELECT * FROM users WHERE id = \" + id)\n}",
		},
	})
	serverIn.WriteString(lspMessage(openReq))
	server.handleMessage(context.Background())

	// Wait for debounce + analysis
	time.Sleep(200 * time.Millisecond)

	// Check diagnostics were published
	output := serverOut.String()
	if !strings.Contains(output, "publishDiagnostics") {
		t.Error("expected publishDiagnostics notification")
	}
	if !strings.Contains(output, "SQL injection") {
		t.Error("expected SQL injection finding in diagnostics")
	}

	server.watcher.Stop()
}

func jsonRPCRequest(id int, method string, params interface{}) string {
	p, _ := json.Marshal(params)
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"%s","params":%s}`, id, method, string(p))
}

func jsonRPCNotification(method string, params interface{}) string {
	p, _ := json.Marshal(params)
	return fmt.Sprintf(`{"jsonrpc":"2.0","method":"%s","params":%s}`, method, string(p))
}

func lspMessage(content string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(content), content)
}
```

**Step 2: Run integration test**

Run: `go test ./internal/lsp/ -run TestLSPIntegration -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/lsp/integration_test.go
git commit -m "$(cat <<'EOF'
test(lsp): add end-to-end integration test

Simulates editor interaction: initialize, open document, receive
diagnostics. Verifies full pipeline from file open to diagnostic
publishing.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Documentation

Document LSP setup for common editors.

**Files:**
- Create: `docs/lsp-setup.md`

**Step 1: Write documentation**

```markdown
# Gavel LSP Setup Guide

Gavel can run as a Language Server Protocol (LSP) server, providing in-editor AI-powered code analysis for any LSP-compatible editor.

## Quick Start

```bash
# Build gavel
task build

# Test LSP mode
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./gavel lsp
```

## Editor Configuration

### Neovim (native LSP)

```lua
-- In your init.lua or after/plugin/lsp.lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "go", "python", "typescript", "javascript" },
  callback = function()
    vim.lsp.start({
      name = "gavel",
      cmd = { "/path/to/gavel", "lsp" },
      root_dir = vim.fn.getcwd(),
    })
  end,
})
```

### Neovim (nvim-lspconfig)

```lua
-- In your LSP config
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

configs.gavel = {
  default_config = {
    cmd = { "/path/to/gavel", "lsp" },
    filetypes = { "go", "python", "typescript", "javascript" },
    root_dir = lspconfig.util.root_pattern(".git", ".gavel"),
  },
}

lspconfig.gavel.setup({})
```

### Helix

Add to `~/.config/helix/languages.toml`:

```toml
[[language]]
name = "go"
language-servers = ["gopls", "gavel"]

[language-server.gavel]
command = "/path/to/gavel"
args = ["lsp"]
```

### VS Code

Create `.vscode/settings.json`:

```json
{
  "lsp.server": {
    "gavel": {
      "command": "/path/to/gavel",
      "args": ["lsp"],
      "filetypes": ["go", "python", "typescript", "javascript"]
    }
  }
}
```

Or use a generic LSP client extension.

## Configuration

Gavel LSP uses the same tiered configuration as CLI mode:

1. System defaults (built-in)
2. Machine config: `~/.config/gavel/policies.yaml`
3. Project config: `.gavel/policies.yaml`

### LSP-specific options

```yaml
# .gavel/policies.yaml
lsp:
  watcher:
    debounce_duration: 5m    # Wait after last file change
    watch_patterns:
      - "**/*.go"
      - "**/*.py"
    ignore_patterns:
      - "**/vendor/**"

  analysis:
    parallel_files: 3        # Concurrent analyses

  cache:
    ttl: 168h                # 7 days
    max_size_mb: 500
```

## How It Works

1. **File opened/saved** → Gavel receives `textDocument/didOpen` or `textDocument/didSave`
2. **Debounce** → Waits for configured duration (default 5m) for more changes
3. **Cache check** → Computes content hash, checks local cache
4. **Analysis** → On cache miss, runs BAML analysis with configured policies
5. **Diagnostics** → Converts SARIF results to LSP diagnostics
6. **Publish** → Sends `textDocument/publishDiagnostics` to editor

## Troubleshooting

### No diagnostics appearing

1. Check gavel is in your PATH or use absolute path
2. Verify config with `./gavel analyze --dir . --dry-run`
3. Check editor LSP logs for connection errors

### Analysis too slow

1. Reduce `debounce_duration` for faster feedback
2. Use a faster model (e.g., Ollama local)
3. Increase `parallel_files` if you have resources

### Cache not working

1. Check `.gavel/cache/local/` exists and is writable
2. Verify cache key components haven't changed (model, policies)
```

**Step 2: Commit**

```bash
git add docs/lsp-setup.md
git commit -m "$(cat <<'EOF'
docs: add LSP setup guide for common editors

Covers neovim, helix, VS Code configuration. Documents LSP-specific
options and troubleshooting tips.

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Run Full Test Suite and Fix Issues

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Run linter**

Run: `go vet ./...`
Expected: No issues

**Step 3: Fix go.mod**

Run: `go mod tidy`
Expected: Dependencies updated

**Step 4: Commit any fixes**

```bash
git add -A
git commit -m "$(cat <<'EOF'
chore: fix test failures and tidy dependencies

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

---

## Summary

This plan implements Phase 2 (LSP Integration) of the TUI/LSP design:

1. **Cache Manager** - Content-addressable caching shared between CLI and LSP
2. **File Watcher** - Debounced file watching to batch analysis requests
3. **Diagnostic Mapper** - SARIF to LSP diagnostic conversion
4. **Protocol Types** - Core LSP message types
5. **LSP Server** - JSON-RPC 2.0 server with document sync
6. **Analyzer Integration** - Wraps BAML analyzer for LSP use
7. **CLI Command** - `gavel lsp` command to start server
8. **Configuration** - LSP-specific config options
9. **Integration Test** - End-to-end LSP test
10. **Documentation** - Editor setup guide

Total estimated: 11 tasks with ~50 bite-sized steps.
