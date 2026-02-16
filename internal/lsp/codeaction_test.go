// internal/lsp/codeaction_test.go
package lsp

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestGetCodeActions(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		diagnostics []Diagnostic
		results     []sarif.Result
		wantCount   int
	}{
		{
			name: "no diagnostics",
			uri:  "file:///test.go",
			diagnostics: []Diagnostic{},
			results:     []sarif.Result{},
			wantCount:   0,
		},
		{
			name: "diagnostic with recommendation",
			uri:  "file:///test.go",
			diagnostics: []Diagnostic{
				{
					Range:    Range{Start: Position{Line: 10}, End: Position{Line: 10}},
					Code:     "test-rule",
					Message:  "Test issue",
					Severity: DiagnosticSeverityWarning,
				},
			},
			results: []sarif.Result{
				{
					RuleID: "test-rule",
					Level:  "warning",
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							Region: sarif.Region{StartLine: 11}, // 1-indexed
						},
					}},
					Properties: map[string]interface{}{
						"gavel/recommendation": "Fix the issue by doing X",
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "diagnostic without recommendation",
			uri:  "file:///test.go",
			diagnostics: []Diagnostic{
				{
					Range:    Range{Start: Position{Line: 10}, End: Position{Line: 10}},
					Code:     "test-rule",
					Message:  "Test issue",
					Severity: DiagnosticSeverityWarning,
				},
			},
			results: []sarif.Result{
				{
					RuleID: "test-rule",
					Level:  "warning",
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							Region: sarif.Region{StartLine: 11},
						},
					}},
					Properties: map[string]interface{}{},
				},
			},
			wantCount: 0,
		},
		{
			name: "diagnostic with recommendation in Data field",
			uri:  "file:///test.go",
			diagnostics: []Diagnostic{
				{
					Range:    Range{Start: Position{Line: 10}, End: Position{Line: 10}},
					Code:     "other-rule",
					Message:  "Another issue",
					Severity: DiagnosticSeverityError,
					Data: &DiagnosticData{
						Recommendation: "Use the Data field recommendation",
					},
				},
			},
			results: []sarif.Result{},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := GetCodeActions(tt.uri, tt.diagnostics, tt.results)
			if len(actions) != tt.wantCount {
				t.Errorf("GetCodeActions() returned %d actions, want %d", len(actions), tt.wantCount)
			}

			// Verify action properties if we got actions
			for _, action := range actions {
				if action.Kind != CodeActionKindQuickFix {
					t.Errorf("Action kind = %s, want %s", action.Kind, CodeActionKindQuickFix)
				}
				if action.Command == nil {
					t.Error("Action should have a command")
				}
			}
		})
	}
}

func TestFilterDiagnosticsForRange(t *testing.T) {
	diagnostics := []Diagnostic{
		{Range: Range{Start: Position{Line: 5}, End: Position{Line: 5}}},
		{Range: Range{Start: Position{Line: 10}, End: Position{Line: 12}}},
		{Range: Range{Start: Position{Line: 20}, End: Position{Line: 25}}},
	}

	tests := []struct {
		name      string
		r         Range
		wantCount int
	}{
		{
			name:      "exact match single line",
			r:         Range{Start: Position{Line: 5}, End: Position{Line: 5}},
			wantCount: 1,
		},
		{
			name:      "overlap with multi-line diagnostic",
			r:         Range{Start: Position{Line: 11}, End: Position{Line: 11}},
			wantCount: 1,
		},
		{
			name:      "no overlap",
			r:         Range{Start: Position{Line: 15}, End: Position{Line: 15}},
			wantCount: 0,
		},
		{
			name:      "range spans multiple diagnostics",
			r:         Range{Start: Position{Line: 5}, End: Position{Line: 25}},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := FilterDiagnosticsForRange(diagnostics, tt.r)
			if len(filtered) != tt.wantCount {
				t.Errorf("FilterDiagnosticsForRange() returned %d diagnostics, want %d", len(filtered), tt.wantCount)
			}
		})
	}
}

func TestTruncateTitle(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"this is a longer string", 10, "this is..."},
		{"ab", 3, "ab"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateTitle(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateTitle(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
