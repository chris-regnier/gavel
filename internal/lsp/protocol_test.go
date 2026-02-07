// internal/lsp/protocol_test.go
package lsp

import (
	"encoding/json"
	"testing"
)

func TestPublishDiagnosticsParams(t *testing.T) {
	// Create a PublishDiagnosticsParams with some diagnostics
	params := PublishDiagnosticsParams{
		URI: "file:///path/to/file.go",
		Diagnostics: []Diagnostic{
			{
				Range: Range{
					Start: Position{Line: 10, Character: 5},
					End:   Position{Line: 10, Character: 15},
				},
				Severity: DiagnosticSeverityError,
				Code:     "SEC001",
				Source:   "gavel",
				Message:  "Security issue found",
				Data: &DiagnosticData{
					Confidence:     0.95,
					Explanation:    "This is a security vulnerability",
					Recommendation: "Fix it this way",
				},
			},
			{
				Range: Range{
					Start: Position{Line: 20, Character: 0},
					End:   Position{Line: 20, Character: 10},
				},
				Severity: DiagnosticSeverityWarning,
				Code:     "STYLE001",
				Source:   "gavel",
				Message:  "Style issue",
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal back
	var decoded PublishDiagnosticsParams
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify round-trip
	if decoded.URI != params.URI {
		t.Errorf("URI mismatch: expected %s, got %s", params.URI, decoded.URI)
	}

	if len(decoded.Diagnostics) != len(params.Diagnostics) {
		t.Fatalf("Diagnostics count mismatch: expected %d, got %d",
			len(params.Diagnostics), len(decoded.Diagnostics))
	}

	// Check first diagnostic
	diag1 := decoded.Diagnostics[0]
	if diag1.Code != "SEC001" {
		t.Errorf("First diagnostic code mismatch: expected SEC001, got %s", diag1.Code)
	}
	if diag1.Severity != DiagnosticSeverityError {
		t.Errorf("First diagnostic severity mismatch: expected %d, got %d",
			DiagnosticSeverityError, diag1.Severity)
	}
	if diag1.Data == nil {
		t.Error("First diagnostic data should not be nil")
	} else {
		if diag1.Data.Confidence != 0.95 {
			t.Errorf("First diagnostic confidence mismatch: expected 0.95, got %f",
				diag1.Data.Confidence)
		}
	}

	// Check second diagnostic
	diag2 := decoded.Diagnostics[1]
	if diag2.Code != "STYLE001" {
		t.Errorf("Second diagnostic code mismatch: expected STYLE001, got %s", diag2.Code)
	}
	if diag2.Severity != DiagnosticSeverityWarning {
		t.Errorf("Second diagnostic severity mismatch: expected %d, got %d",
			DiagnosticSeverityWarning, diag2.Severity)
	}
}

func TestMethodConstants(t *testing.T) {
	// Verify method constant values match LSP spec
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Initialize", MethodInitialize, "initialize"},
		{"Initialized", MethodInitialized, "initialized"},
		{"Shutdown", MethodShutdown, "shutdown"},
		{"Exit", MethodExit, "exit"},
		{"DidOpen", MethodTextDocumentDidOpen, "textDocument/didOpen"},
		{"DidClose", MethodTextDocumentDidClose, "textDocument/didClose"},
		{"DidSave", MethodTextDocumentDidSave, "textDocument/didSave"},
		{"PublishDiagnostics", MethodTextDocumentPublishDiagnostics, "textDocument/publishDiagnostics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.constant)
			}
		})
	}
}
