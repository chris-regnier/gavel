// internal/lsp/diagnostic.go
package lsp

import (
	"github.com/chris-regnier/gavel/internal/sarif"
)

// DiagnosticSeverity maps to LSP severity levels
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

// Range represents a text range in a document
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// DiagnosticData holds gavel-specific diagnostic metadata
type DiagnosticData struct {
	Confidence     float64 `json:"confidence,omitempty"`
	Explanation    string  `json:"explanation,omitempty"`
	Recommendation string  `json:"recommendation,omitempty"`
}

// Diagnostic represents an LSP diagnostic message
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity"`
	Code     string             `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
	Data     *DiagnosticData    `json:"data,omitempty"`
}

// levelToSeverity maps SARIF level strings to LSP severity
func levelToSeverity(level string) DiagnosticSeverity {
	switch level {
	case "error":
		return DiagnosticSeverityError
	case "warning":
		return DiagnosticSeverityWarning
	case "note":
		return DiagnosticSeverityInformation
	default:
		return DiagnosticSeverityInformation
	}
}

// SarifToDiagnostic converts a SARIF result to an LSP diagnostic
func SarifToDiagnostic(result sarif.Result) Diagnostic {
	diag := Diagnostic{
		Severity: levelToSeverity(result.Level),
		Code:     result.RuleID,
		Source:   "gavel",
		Message:  result.Message.Text,
	}

	// Extract location information
	if len(result.Locations) > 0 {
		loc := result.Locations[0]
		region := loc.PhysicalLocation.Region

		// SARIF uses 1-indexed lines, LSP uses 0-indexed
		startLine := region.StartLine - 1
		endLine := region.EndLine - 1

		// Ensure non-negative line numbers
		if startLine < 0 {
			startLine = 0
		}
		if endLine < 0 {
			endLine = 0
		}

		diag.Range = Range{
			Start: Position{
				Line:      startLine,
				Character: 0,
			},
			End: Position{
				Line:      endLine,
				Character: 0,
			},
		}
	}

	// Extract gavel-specific properties
	if result.Properties != nil {
		data := &DiagnosticData{}
		hasData := false

		if confidence, ok := result.Properties["gavel/confidence"].(float64); ok {
			data.Confidence = confidence
			hasData = true
		}
		if explanation, ok := result.Properties["gavel/explanation"].(string); ok {
			data.Explanation = explanation
			hasData = true
		}
		if recommendation, ok := result.Properties["gavel/recommendation"].(string); ok {
			data.Recommendation = recommendation
			hasData = true
		}

		if hasData {
			diag.Data = data
		}
	}

	return diag
}

// SarifResultsToDiagnostics converts multiple SARIF results to LSP diagnostics
func SarifResultsToDiagnostics(results []sarif.Result) []Diagnostic {
	diagnostics := make([]Diagnostic, 0, len(results))
	for _, result := range results {
		diagnostics = append(diagnostics, SarifToDiagnostic(result))
	}
	return diagnostics
}
