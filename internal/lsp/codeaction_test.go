// internal/lsp/codeaction_test.go
package lsp

import (
	"strings"
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
		wantCommand bool
		wantEdit    bool
	}{
		{
			name:        "no diagnostics",
			uri:         "file:///test.go",
			diagnostics: []Diagnostic{},
			results:     []sarif.Result{},
			wantCount:   0,
		},
		{
			name: "diagnostic with recommendation falls back to command",
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
					Properties: map[string]interface{}{
						"gavel/recommendation": "Fix the issue by doing X",
					},
				},
			},
			wantCount:   1,
			wantCommand: true,
		},
		{
			name: "diagnostic without recommendation produces no action",
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
			name: "diagnostic with recommendation in Data field falls back to command",
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
			results:     []sarif.Result{},
			wantCount:   1,
			wantCommand: true,
		},
		{
			name: "diagnostic with structured Fix produces WorkspaceEdit",
			uri:  "file:///workspace/config.go",
			diagnostics: []Diagnostic{
				{
					Range:    Range{Start: Position{Line: 41}, End: Position{Line: 41}},
					Code:     "S2068",
					Message:  "Hardcoded credential",
					Severity: DiagnosticSeverityError,
				},
			},
			results: []sarif.Result{
				{
					RuleID: "S2068",
					Level:  "error",
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "config.go"},
							Region:           sarif.Region{StartLine: 42},
						},
					}},
					Properties: map[string]interface{}{
						"gavel/recommendation": "Replace hardcoded credential",
					},
					Fixes: []sarif.Fix{{
						Description: sarif.Message{Text: "Use environment variable"},
						ArtifactChanges: []sarif.ArtifactChange{{
							ArtifactLocation: sarif.ArtifactLocation{URI: "config.go"},
							Replacements: []sarif.Replacement{{
								DeletedRegion: sarif.Region{StartLine: 42, EndLine: 42},
								InsertedContent: &sarif.ArtifactContent{
									Text: `os.Getenv("DB_PASSWORD")`,
								},
							}},
						}},
					}},
				},
			},
			wantCount: 1,
			wantEdit:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := GetCodeActions(tt.uri, tt.diagnostics, tt.results)
			if len(actions) != tt.wantCount {
				t.Errorf("GetCodeActions() returned %d actions, want %d", len(actions), tt.wantCount)
			}

			for _, action := range actions {
				if action.Kind != CodeActionKindQuickFix {
					t.Errorf("Action kind = %s, want %s", action.Kind, CodeActionKindQuickFix)
				}
				if tt.wantCommand && action.Command == nil {
					t.Error("Action should have a command")
				}
				if tt.wantEdit {
					if action.Edit == nil {
						t.Fatal("Action should have an edit")
					}
					if action.Command != nil {
						t.Error("Action with edit should not also carry a command")
					}
					if !action.IsPreferred {
						t.Error("Edit-bearing action should be marked IsPreferred")
					}
				}
			}
		})
	}
}

func TestGetCodeActions_FixTitlePrefersDescription(t *testing.T) {
	uri := "file:///workspace/main.go"
	diagnostics := []Diagnostic{{
		Range:   Range{Start: Position{Line: 9}, End: Position{Line: 9}},
		Code:    "RULE1",
		Message: "Issue",
	}}
	results := []sarif.Result{{
		RuleID: "RULE1",
		Locations: []sarif.Location{{
			PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
				Region:           sarif.Region{StartLine: 10},
			},
		}},
		Properties: map[string]interface{}{
			"gavel/recommendation": "Generic recommendation text",
		},
		Fixes: []sarif.Fix{{
			Description: sarif.Message{Text: "Specific fix description"},
			ArtifactChanges: []sarif.ArtifactChange{{
				ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
				Replacements: []sarif.Replacement{{
					DeletedRegion:   sarif.Region{StartLine: 10, EndLine: 10},
					InsertedContent: &sarif.ArtifactContent{Text: "newCode"},
				}},
			}},
		}},
	}}

	actions := GetCodeActions(uri, diagnostics, results)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0].Title, "Specific fix description") {
		t.Errorf("title %q should derive from Fix.Description, not the recommendation", actions[0].Title)
	}
}

func TestGetCodeActions_FixWithoutDescriptionUsesRecommendation(t *testing.T) {
	uri := "file:///workspace/main.go"
	diagnostics := []Diagnostic{{
		Range: Range{Start: Position{Line: 9}, End: Position{Line: 9}},
		Code:  "RULE1",
	}}
	results := []sarif.Result{{
		RuleID: "RULE1",
		Locations: []sarif.Location{{
			PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
				Region:           sarif.Region{StartLine: 10},
			},
		}},
		Properties: map[string]interface{}{
			"gavel/recommendation": "Use the recommendation",
		},
		Fixes: []sarif.Fix{{
			ArtifactChanges: []sarif.ArtifactChange{{
				ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
				Replacements: []sarif.Replacement{{
					DeletedRegion:   sarif.Region{StartLine: 10, EndLine: 10},
					InsertedContent: &sarif.ArtifactContent{Text: "x"},
				}},
			}},
		}},
	}}

	actions := GetCodeActions(uri, diagnostics, results)
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0].Title, "Use the recommendation") {
		t.Errorf("title %q should fall back to the recommendation when description is empty", actions[0].Title)
	}
}

func TestBuildWorkspaceEdit_MultiFile(t *testing.T) {
	docURI := "file:///workspace/handler.go"
	fix := sarif.Fix{
		ArtifactChanges: []sarif.ArtifactChange{
			{
				ArtifactLocation: sarif.ArtifactLocation{URI: "handler.go"},
				Replacements: []sarif.Replacement{{
					DeletedRegion:   sarif.Region{StartLine: 5, EndLine: 5},
					InsertedContent: &sarif.ArtifactContent{Text: "sanitized"},
				}},
			},
			{
				ArtifactLocation: sarif.ArtifactLocation{URI: "/workspace/db/query.go"},
				Replacements: []sarif.Replacement{{
					DeletedRegion:   sarif.Region{StartLine: 12, EndLine: 12},
					InsertedContent: &sarif.ArtifactContent{Text: "?"},
				}},
			},
		},
	}

	edit := buildWorkspaceEdit(docURI, fix)
	if edit == nil {
		t.Fatal("expected non-nil WorkspaceEdit")
	}
	if len(edit.Changes) != 2 {
		t.Fatalf("expected 2 file entries, got %d: %v", len(edit.Changes), keys(edit.Changes))
	}
	if _, ok := edit.Changes[docURI]; !ok {
		t.Errorf("expected document URI %q in Changes, got %v", docURI, keys(edit.Changes))
	}
	if _, ok := edit.Changes["file:///workspace/db/query.go"]; !ok {
		t.Errorf("expected absolute artifact URI to be promoted to file:// URI, got %v", keys(edit.Changes))
	}
}

func TestSarifRegionToLSPRange(t *testing.T) {
	tests := []struct {
		name   string
		region sarif.Region
		want   Range
	}{
		{
			name:   "single line",
			region: sarif.Region{StartLine: 42, EndLine: 42},
			want: Range{
				Start: Position{Line: 41, Character: 0},
				End:   Position{Line: 42, Character: 0},
			},
		},
		{
			name:   "multi-line",
			region: sarif.Region{StartLine: 10, EndLine: 15},
			want: Range{
				Start: Position{Line: 9, Character: 0},
				End:   Position{Line: 15, Character: 0},
			},
		},
		{
			name:   "missing end line collapses to start",
			region: sarif.Region{StartLine: 5},
			want: Range{
				Start: Position{Line: 4, Character: 0},
				End:   Position{Line: 5, Character: 0},
			},
		},
		{
			name:   "zero start clamps to first line",
			region: sarif.Region{StartLine: 0, EndLine: 1},
			want: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 1, Character: 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sarifRegionToLSPRange(tt.region)
			if got != tt.want {
				t.Errorf("sarifRegionToLSPRange(%+v) = %+v, want %+v", tt.region, got, tt.want)
			}
		})
	}
}

func TestResolveArtifactURI(t *testing.T) {
	tests := []struct {
		name        string
		documentURI string
		artifactURI string
		want        string
	}{
		{
			name:        "empty artifact URI reuses document",
			documentURI: "file:///workspace/main.go",
			artifactURI: "",
			want:        "file:///workspace/main.go",
		},
		{
			name:        "already a file URI returned verbatim",
			documentURI: "file:///workspace/main.go",
			artifactURI: "file:///elsewhere/other.go",
			want:        "file:///elsewhere/other.go",
		},
		{
			name:        "matching basename reuses document URI",
			documentURI: "file:///workspace/main.go",
			artifactURI: "main.go",
			want:        "file:///workspace/main.go",
		},
		{
			name:        "matching subpath reuses document URI",
			documentURI: "file:///workspace/pkg/main.go",
			artifactURI: "pkg/main.go",
			want:        "file:///workspace/pkg/main.go",
		},
		{
			name:        "absolute path gets file:// prefix",
			documentURI: "file:///workspace/main.go",
			artifactURI: "/other/path/util.go",
			want:        "file:///other/path/util.go",
		},
		{
			name:        "relative path resolves against document directory",
			documentURI: "file:///workspace/pkg/main.go",
			artifactURI: "../util/helper.go",
			want:        "file:///workspace/util/helper.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveArtifactURI(tt.documentURI, tt.artifactURI)
			if got != tt.want {
				t.Errorf("resolveArtifactURI(%q, %q) = %q, want %q", tt.documentURI, tt.artifactURI, got, tt.want)
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

func keys(m map[string][]TextEdit) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
