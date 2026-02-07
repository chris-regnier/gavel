// internal/lsp/diagnostic_test.go
package lsp

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestSarifToDiagnostic(t *testing.T) {
	result := sarif.Result{
		RuleID: "SEC001",
		Level:  "error",
		Message: sarif.Message{
			Text: "Potential SQL injection vulnerability",
		},
		Locations: []sarif.Location{
			{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{
						URI: "src/main.go",
					},
					Region: sarif.Region{
						StartLine: 42,
						EndLine:   43,
					},
				},
			},
		},
		Properties: map[string]interface{}{
			"gavel/confidence":     0.95,
			"gavel/explanation":    "Direct string concatenation in SQL query",
			"gavel/recommendation": "Use parameterized queries",
		},
	}

	diag := SarifToDiagnostic(result)

	// Check basic fields
	if diag.Severity != DiagnosticSeverityError {
		t.Errorf("Expected severity %d (Error), got %d", DiagnosticSeverityError, diag.Severity)
	}
	if diag.Code != "SEC001" {
		t.Errorf("Expected code 'SEC001', got '%s'", diag.Code)
	}
	if diag.Source != "gavel" {
		t.Errorf("Expected source 'gavel', got '%s'", diag.Source)
	}
	if diag.Message != "Potential SQL injection vulnerability" {
		t.Errorf("Expected message 'Potential SQL injection vulnerability', got '%s'", diag.Message)
	}

	// Check range - SARIF uses 1-indexed lines, LSP uses 0-indexed
	if diag.Range.Start.Line != 41 {
		t.Errorf("Expected start line 41 (SARIF 42 - 1), got %d", diag.Range.Start.Line)
	}
	if diag.Range.End.Line != 42 {
		t.Errorf("Expected end line 42 (SARIF 43 - 1), got %d", diag.Range.End.Line)
	}
	if diag.Range.Start.Character != 0 {
		t.Errorf("Expected start character 0, got %d", diag.Range.Start.Character)
	}
	if diag.Range.End.Character != 0 {
		t.Errorf("Expected end character 0, got %d", diag.Range.End.Character)
	}

	// Check data field
	if diag.Data == nil {
		t.Fatal("Expected Data to be non-nil")
	}
	if diag.Data.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", diag.Data.Confidence)
	}
	if diag.Data.Explanation != "Direct string concatenation in SQL query" {
		t.Errorf("Expected explanation, got '%s'", diag.Data.Explanation)
	}
	if diag.Data.Recommendation != "Use parameterized queries" {
		t.Errorf("Expected recommendation, got '%s'", diag.Data.Recommendation)
	}
}

func TestSarifToDiagnosticWarning(t *testing.T) {
	result := sarif.Result{
		RuleID: "STYLE001",
		Level:  "warning",
		Message: sarif.Message{
			Text: "Variable name is too short",
		},
		Locations: []sarif.Location{
			{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{
						URI: "src/util.go",
					},
					Region: sarif.Region{
						StartLine: 10,
						EndLine:   10,
					},
				},
			},
		},
		Properties: map[string]interface{}{
			"gavel/confidence":     0.8,
			"gavel/explanation":    "Single character variable names reduce readability",
			"gavel/recommendation": "Use descriptive variable names",
		},
	}

	diag := SarifToDiagnostic(result)

	if diag.Severity != DiagnosticSeverityWarning {
		t.Errorf("Expected severity %d (Warning), got %d", DiagnosticSeverityWarning, diag.Severity)
	}
	if diag.Range.Start.Line != 9 {
		t.Errorf("Expected start line 9 (SARIF 10 - 1), got %d", diag.Range.Start.Line)
	}
}

func TestSarifResultsToDiagnostics(t *testing.T) {
	results := []sarif.Result{
		{
			RuleID: "SEC001",
			Level:  "error",
			Message: sarif.Message{
				Text: "Security issue",
			},
			Locations: []sarif.Location{
				{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{
							URI: "main.go",
						},
						Region: sarif.Region{
							StartLine: 1,
							EndLine:   1,
						},
					},
				},
			},
		},
		{
			RuleID: "STYLE001",
			Level:  "note",
			Message: sarif.Message{
				Text: "Style suggestion",
			},
			Locations: []sarif.Location{
				{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{
							URI: "util.go",
						},
						Region: sarif.Region{
							StartLine: 5,
							EndLine:   5,
						},
					},
				},
			},
		},
	}

	diagnostics := SarifResultsToDiagnostics(results)

	if len(diagnostics) != 2 {
		t.Fatalf("Expected 2 diagnostics, got %d", len(diagnostics))
	}

	if diagnostics[0].Code != "SEC001" {
		t.Errorf("Expected first diagnostic code 'SEC001', got '%s'", diagnostics[0].Code)
	}
	if diagnostics[1].Code != "STYLE001" {
		t.Errorf("Expected second diagnostic code 'STYLE001', got '%s'", diagnostics[1].Code)
	}
}
