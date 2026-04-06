// internal/lsp/server_test.go
package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// jsonRPCRequest represents a JSON-RPC 2.0 request
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// Helper to create JSON-RPC message
func makeJSONRPCMessage(method string, params interface{}, id int) string {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, _ := json.Marshal(req)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(data), data)
}

func TestServerInitialize(t *testing.T) {
	// Create input with initialize request
	initParams := InitializeParams{
		ProcessID: intPtr(12345),
		RootURI:   "file:///workspace",
		Capabilities: ClientCapabilities{
			TextDocument: &TextDocumentClientCapabilities{
				PublishDiagnostics: &PublishDiagnosticsClientCapabilities{
					RelatedInformation: true,
				},
			},
		},
	}

	input := makeJSONRPCMessage(MethodInitialize, initParams, 1)

	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	// Mock analyze function that returns empty results
	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		return []sarif.Result{}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)

	// Run server in goroutine with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	serverDone := make(chan struct{})
	go func() {
		server.Run(ctx)
		close(serverDone)
	}()

	// Wait for server goroutine to finish (input EOF causes immediate exit)
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit in time")
	}

	writer.Flush()

	// Parse response
	outputStr := output.String()
	if !strings.Contains(outputStr, "Content-Length:") {
		t.Fatalf("Expected Content-Length header in response, got: %s", outputStr)
	}

	// Extract JSON from response
	parts := strings.Split(outputStr, "\r\n\r\n")
	if len(parts) < 2 {
		t.Fatalf("Invalid response format: %s", outputStr)
	}

	var response jsonRPCResponse
	if err := json.Unmarshal([]byte(parts[1]), &response); err != nil {
		t.Fatalf("Failed to parse response JSON: %v\nResponse: %s", err, parts[1])
	}

	// Verify response
	if response.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC 2.0, got %s", response.JSONRPC)
	}
	if response.Error != nil {
		t.Errorf("Expected no error, got %v", response.Error)
	}

	// Check result contains capabilities
	resultJSON, _ := json.Marshal(response.Result)
	var initResult InitializeResult
	if err := json.Unmarshal(resultJSON, &initResult); err != nil {
		t.Fatalf("Failed to parse InitializeResult: %v", err)
	}

	if initResult.Capabilities.TextDocumentSync == nil {
		t.Error("Expected TextDocumentSync capabilities")
	}
	if initResult.ServerInfo == nil {
		t.Error("Expected ServerInfo")
	}
	if initResult.ServerInfo.Name != "gavel-lsp" {
		t.Errorf("Expected server name 'gavel-lsp', got '%s'", initResult.ServerInfo.Name)
	}
}

func TestServerDocumentSync(t *testing.T) {
	// Create didOpen request
	didOpenParams := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.go",
			LanguageID: "go",
			Version:    1,
			Text:       "package main\n\nfunc main() {}\n",
		},
	}

	input := makeJSONRPCMessage(MethodTextDocumentDidOpen, didOpenParams, 2)

	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	// Track if analyze was called
	var analyzeCalled atomic.Bool
	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		analyzeCalled.Store(true)
		if path != "/test.go" {
			t.Errorf("Expected path '/test.go', got '%s'", path)
		}
		if !strings.Contains(content, "package main") {
			t.Errorf("Expected content to contain 'package main', got: %s", content)
		}
		return []sarif.Result{}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		server.Run(ctx)
	}()

	// Wait for debounced watcher to trigger (300ms debounce + buffer)
	time.Sleep(400 * time.Millisecond)

	// Cancel server before reading shared state to avoid data race
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Verify document was tracked
	if !analyzeCalled.Load() {
		t.Error("Expected analyze function to be called after didOpen")
	}
}

func TestServerSkipsUnchangedFile(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"

	didOpenParams := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.go",
			LanguageID: "go",
			Version:    1,
			Text:       content,
		},
	}

	didSaveParams := DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///test.go"},
		Text:         &content,
	}

	input := makeJSONRPCMessage(MethodTextDocumentDidOpen, didOpenParams, 1) +
		makeJSONRPCMessage(MethodTextDocumentDidSave, didSaveParams, 2)

	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	var analyzeCount atomic.Int32
	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		analyzeCount.Add(1)
		return []sarif.Result{}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	go server.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	// Cancel server before reading shared state to avoid data race
	cancel()
	time.Sleep(50 * time.Millisecond)

	if analyzeCount.Load() != 1 {
		t.Errorf("Expected 1 analysis call (skip unchanged), got %d", analyzeCount.Load())
	}
}

func TestServerProgressiveMultipleTiers(t *testing.T) {
	didOpenParams := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.go",
			LanguageID: "go",
			Version:    1,
			Text:       "package main\n\nfunc main() { var x = 1 }\n",
		},
	}

	input := makeJSONRPCMessage(MethodTextDocumentDidOpen, didOpenParams, 1)

	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(input))
	writer := bufio.NewWriter(&output)

	syncFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		return nil, nil
	}

	server := NewServer(reader, writer, syncFunc)

	server.SetProgressiveAnalyze(func(ctx context.Context, path, content string) <-chan ProgressiveResult {
		ch := make(chan ProgressiveResult, 2)
		go func() {
			defer close(ch)
			ch <- ProgressiveResult{
				Tier: "instant",
				Results: []sarif.Result{{
					RuleID:  "PAT001",
					Level:   "warning",
					Message: sarif.Message{Text: "pattern match"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: path},
							Region:           sarif.Region{StartLine: 3, EndLine: 3},
						},
					}},
					Properties: map[string]interface{}{"gavel/tier": "instant"},
				}},
			}
			time.Sleep(50 * time.Millisecond)
			ch <- ProgressiveResult{
				Tier: "comprehensive",
				Results: []sarif.Result{{
					RuleID:  "LLM001",
					Level:   "error",
					Message: sarif.Message{Text: "unused variable"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: path},
							Region:           sarif.Region{StartLine: 3, EndLine: 3},
						},
					}},
					Properties: map[string]interface{}{"gavel/tier": "comprehensive"},
				}},
			}
		}()
		return ch
	})

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	serverDone := make(chan struct{})
	go func() {
		server.Run(ctx)
		close(serverDone)
	}()

	// Wait for debounce + analysis to complete
	time.Sleep(700 * time.Millisecond)

	// Stop watcher and cancel server, then wait for goroutine to finish
	server.watcher.Stop()
	cancel()
	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not exit in time")
	}

	// Lock writerMu to synchronize with any in-flight analysis goroutines,
	// then read output safely. sendMessage already flushes after each write.
	server.writerMu.Lock()
	writer.Flush()
	outputStr := output.String()
	server.writerMu.Unlock()

	publishCount := strings.Count(outputStr, MethodTextDocumentPublishDiagnostics)
	if publishCount < 2 {
		t.Errorf("Expected at least 2 publishDiagnostics notifications (one per tier), got %d", publishCount)
	}
}

func intPtr(i int) *int {
	return &i
}
