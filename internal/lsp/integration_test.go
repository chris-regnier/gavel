package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestLSPIntegration(t *testing.T) {
	// Setup mock analyzer with findings
	analyzeCalled := false
	mockAnalyze := func(ctx context.Context, path, content string) ([]sarif.Result, error) {
		analyzeCalled = true
		// Only return findings if the content contains SQL injection vulnerability
		if strings.Contains(content, "SELECT * FROM users WHERE id = ' + userId") {
			return []sarif.Result{
				{
					RuleID:  "sql-injection",
					Level:   "error",
					Message: sarif.Message{Text: "SQL injection vulnerability detected"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: path},
							Region:           sarif.Region{StartLine: 5, EndLine: 5},
						},
					}},
				},
			}, nil
		}
		return []sarif.Result{}, nil
	}

	// Code with SQL injection vulnerability
	codeWithVuln := `package main

func getUser(userId string) {
    // SQL injection vulnerability
    query := "SELECT * FROM users WHERE id = ' + userId"
    db.Query(query)
}
`

	// Create input messages
	var inputBuf strings.Builder

	// Initialize request
	inputBuf.WriteString(makeJSONRPCMessage(MethodInitialize, InitializeParams{
		RootURI: "file:///test",
	}, 1))

	// Initialized notification
	inputBuf.WriteString(makeJSONRPCNotification(MethodInitialized, map[string]interface{}{}))

	// DidOpen with vulnerable code
	inputBuf.WriteString(makeJSONRPCNotification(MethodTextDocumentDidOpen, DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test/main.go",
			LanguageID: "go",
			Version:    1,
			Text:       codeWithVuln,
		},
	}))

	var outputBuf bytes.Buffer
	reader := bufio.NewReader(strings.NewReader(inputBuf.String()))
	writer := bufio.NewWriter(&outputBuf)

	// Create server with short debounce for testing
	server := NewServer(reader, writer, mockAnalyze)
	// Override watcher with shorter debounce for testing
	server.watcher = NewDebouncedWatcher(100*time.Millisecond, func(files []string) {
		for _, uri := range files {
			if content, ok := server.documents[uri]; ok {
				path := uriToPath(uri)
				server.analyzeAndPublish(context.Background(), uri, path, content)
			}
		}
	})

	// Start server in background with longer timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx)
	}()

	// Wait for messages to be processed and analysis to complete
	// Initialize (50ms) + didOpen (50ms) + debounce (100ms) + analysis (50ms) = ~250ms
	time.Sleep(300 * time.Millisecond)
	writer.Flush()

	// Get output
	output := outputBuf.String()

	// Verify initialize response was sent
	if !strings.Contains(output, "gavel-lsp") {
		t.Errorf("expected initialize response with server name, got: %s", output)
	}

	// Verify analyze was called
	if !analyzeCalled {
		t.Error("expected analyze to be called")
	}

	// Verify publishDiagnostics notification was sent
	if !strings.Contains(output, MethodTextDocumentPublishDiagnostics) {
		t.Errorf("expected publishDiagnostics notification, got: %s", output)
	}

	// Verify diagnostic contains SQL injection
	if !strings.Contains(output, "SQL injection") {
		t.Errorf("expected diagnostic to contain 'SQL injection', got: %s", output)
	}

	// Parse the publishDiagnostics message to verify structure
	lines := strings.Split(output, "\r\n\r\n")
	foundDiagnostic := false
	for _, line := range lines {
		if strings.Contains(line, MethodTextDocumentPublishDiagnostics) {
			var msg jsonRPCMessage
			if err := json.Unmarshal([]byte(line), &msg); err == nil {
				var params PublishDiagnosticsParams
				if err := json.Unmarshal(msg.Params, &params); err == nil {
					if len(params.Diagnostics) > 0 {
						foundDiagnostic = true
						diag := params.Diagnostics[0]
						if diag.Severity != DiagnosticSeverityError {
							t.Errorf("expected error severity, got %d", diag.Severity)
						}
						// Line 5 in SARIF becomes line 4 in LSP (0-indexed)
						if diag.Range.Start.Line != 4 {
							t.Errorf("expected line 4, got %d", diag.Range.Start.Line)
						}
					}
				}
			}
		}
	}

	if !foundDiagnostic {
		t.Errorf("did not find valid diagnostic in output: %s", output)
	}

	cancel()

	// Wait for server to finish
	select {
	case <-serverDone:
		// Server exited normally
	case <-time.After(500 * time.Millisecond):
		t.Error("server did not exit in time")
	}
}

// Helper function for notifications (no ID field)
func makeJSONRPCNotification(method string, params interface{}) string {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	data, _ := json.Marshal(msg)
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(data), data)
}
