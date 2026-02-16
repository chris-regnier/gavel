// internal/lsp/codeaction.go
package lsp

import (
	"github.com/chris-regnier/gavel/internal/sarif"
)

// GetCodeActions returns code actions for the given diagnostics
func GetCodeActions(uri string, diagnostics []Diagnostic, sarifResults []sarif.Result) []CodeAction {
	var actions []CodeAction

	for _, diag := range diagnostics {
		// Find the corresponding SARIF result to get the recommendation
		recommendation := findRecommendation(diag, sarifResults)
		if recommendation == "" {
			continue
		}

		action := CodeAction{
			Title:       "Gavel: " + truncateTitle(recommendation, 60),
			Kind:        CodeActionKindQuickFix,
			Diagnostics: []Diagnostic{diag},
			Command: &Command{
				Title:   "View recommendation",
				Command: "gavel.showRecommendation",
				Arguments: []interface{}{
					uri,
					diag.Code,
					recommendation,
				},
			},
		}
		actions = append(actions, action)
	}

	return actions
}

// findRecommendation finds the recommendation for a diagnostic from SARIF results
func findRecommendation(diag Diagnostic, results []sarif.Result) string {
	for _, r := range results {
		if r.RuleID != diag.Code {
			continue
		}

		// Check if line matches
		if len(r.Locations) > 0 {
			region := r.Locations[0].PhysicalLocation.Region
			// SARIF is 1-indexed, LSP diagnostic range is 0-indexed
			if region.StartLine-1 != diag.Range.Start.Line {
				continue
			}
		}

		// Extract recommendation from properties
		if r.Properties != nil {
			if rec, ok := r.Properties["gavel/recommendation"].(string); ok {
				return rec
			}
		}
	}

	// Fall back to diagnostic data if available
	if diag.Data != nil && diag.Data.Recommendation != "" {
		return diag.Data.Recommendation
	}

	return ""
}

// truncateTitle truncates a string to maxLen, adding ellipsis if needed
func truncateTitle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// FilterDiagnosticsForRange returns diagnostics that overlap with the given range
func FilterDiagnosticsForRange(diagnostics []Diagnostic, r Range) []Diagnostic {
	var filtered []Diagnostic
	for _, d := range diagnostics {
		if rangesOverlap(d.Range, r) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// rangesOverlap checks if two ranges overlap
func rangesOverlap(a, b Range) bool {
	// Range a ends before b starts
	if a.End.Line < b.Start.Line || (a.End.Line == b.Start.Line && a.End.Character < b.Start.Character) {
		return false
	}
	// Range b ends before a starts
	if b.End.Line < a.Start.Line || (b.End.Line == a.Start.Line && b.End.Character < a.Start.Character) {
		return false
	}
	return true
}
