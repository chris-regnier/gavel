// internal/lsp/commands_test.go
package lsp

import (
	"bufio"
	"bytes"
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestCommandHandler_Execute(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		args       []interface{}
		wantErr    bool
		wantResult bool
	}{
		{
			name:       "unknown command",
			command:    "unknown.command",
			args:       nil,
			wantErr:    true,
			wantResult: false,
		},
		{
			name:       "analyzeFile without args",
			command:    CommandAnalyzeFile,
			args:       nil,
			wantErr:    false,
			wantResult: true, // Returns result with success=false
		},
		{
			name:       "clearCache without cache",
			command:    CommandClearCache,
			args:       nil,
			wantErr:    false,
			wantResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal server for testing
			var output bytes.Buffer
			reader := bufio.NewReader(bytes.NewReader(nil))
			writer := bufio.NewWriter(&output)

			analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
				return []sarif.Result{}, nil
			}

			server := NewServer(reader, writer, analyzeFunc)
			handler := NewCommandHandler(server)

			params := ExecuteCommandParams{
				Command:   tt.command,
				Arguments: tt.args,
			}

			result, err := handler.Execute(context.Background(), params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantResult && result == nil {
				t.Error("Execute() returned nil result, want non-nil")
			}
		})
	}
}

func TestCommandHandler_AnalyzeFile(t *testing.T) {
	var output bytes.Buffer
	reader := bufio.NewReader(bytes.NewReader(nil))
	writer := bufio.NewWriter(&output)

	analyzed := false
	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		analyzed = true
		return []sarif.Result{}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)
	handler := NewCommandHandler(server)

	// Add a document to the server
	server.docMu.Lock()
	server.documents["file:///test.go"] = "package main"
	server.docMu.Unlock()

	params := ExecuteCommandParams{
		Command:   CommandAnalyzeFile,
		Arguments: []interface{}{"file:///test.go"},
	}

	result, err := handler.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cmdResult, ok := result.(*CommandResult)
	if !ok {
		t.Fatalf("Expected *CommandResult, got %T", result)
	}

	if !cmdResult.Success {
		t.Errorf("Expected success=true, got false: %s", cmdResult.Message)
	}

	if !analyzed {
		t.Error("analyzeFunc was not called")
	}
}

func TestCommandHandler_AnalyzeWorkspace(t *testing.T) {
	var output bytes.Buffer
	reader := bufio.NewReader(bytes.NewReader(nil))
	writer := bufio.NewWriter(&output)

	analyzeCount := 0
	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		analyzeCount++
		return []sarif.Result{}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)
	handler := NewCommandHandler(server)

	// Add multiple documents
	server.docMu.Lock()
	server.documents["file:///test1.go"] = "package main"
	server.documents["file:///test2.go"] = "package main"
	server.docMu.Unlock()

	params := ExecuteCommandParams{
		Command:   CommandAnalyzeWorkspace,
		Arguments: nil,
	}

	result, err := handler.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cmdResult, ok := result.(*CommandResult)
	if !ok {
		t.Fatalf("Expected *CommandResult, got %T", result)
	}

	if !cmdResult.Success {
		t.Errorf("Expected success=true, got false: %s", cmdResult.Message)
	}

	if analyzeCount != 2 {
		t.Errorf("Expected 2 analyses, got %d", analyzeCount)
	}
}

func TestCommandHandler_ClearCache(t *testing.T) {
	var output bytes.Buffer
	reader := bufio.NewReader(bytes.NewReader(nil))
	writer := bufio.NewWriter(&output)

	analyzeFunc := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		return []sarif.Result{}, nil
	}

	server := NewServer(reader, writer, analyzeFunc)
	handler := NewCommandHandler(server)

	// Add some cached results
	server.resultsMu.Lock()
	server.resultsCache["file:///test.go"] = resultsCacheEntry{
		results:     []sarif.Result{},
		diagnostics: []Diagnostic{},
	}
	server.resultsMu.Unlock()

	params := ExecuteCommandParams{
		Command:   CommandClearCache,
		Arguments: nil,
	}

	result, err := handler.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cmdResult, ok := result.(*CommandResult)
	if !ok {
		t.Fatalf("Expected *CommandResult, got %T", result)
	}

	// Without cacheManager set, the command returns failure and doesn't clear resultsCache
	if cmdResult.Success {
		t.Error("Expected success=false when cacheManager is nil")
	}
	if cmdResult.Message != "Cache not configured" {
		t.Errorf("Expected message 'Cache not configured', got %q", cmdResult.Message)
	}
}
