package output

import (
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// testMarkdownLog builds a SARIF log with multiple findings across files and
// severities, suitable for exercising the markdown formatter.
func testMarkdownLog() *sarif.Log {
	return &sarif.Log{
		Schema:  sarif.SchemaURI,
		Version: sarif.Version,
		Runs: []sarif.Run{{
			Tool: sarif.Tool{
				Driver: sarif.Driver{
					Name:    "gavel",
					Version: "0.1.0",
				},
			},
			Results: []sarif.Result{
				{
					RuleID:  "SEC001",
					Level:   "error",
					Message: sarif.Message{Text: "Hardcoded database password found in configuration file."},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
							Region:           sarif.Region{StartLine: 42, EndLine: 42},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence":     0.95,
						"gavel/recommendation": "Use environment variables or a secrets manager.",
					},
				},
				{
					RuleID:  "QUAL002",
					Level:   "warning",
					Message: sarif.Message{Text: "Function exceeds 50 lines."},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "internal/handler.go"},
							Region:           sarif.Region{StartLine: 10, EndLine: 75},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence":     0.80,
						"gavel/recommendation": "Refactor into smaller helper functions.",
					},
				},
				{
					RuleID:  "QUAL003",
					Level:   "warning",
					Message: sarif.Message{Text: "Missing error check on Close()."},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "internal/handler.go"},
							Region:           sarif.Region{StartLine: 88, EndLine: 88},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence":     0.70,
						"gavel/recommendation": "Always check the error returned by Close().",
					},
				},
				{
					RuleID:  "QUAL001",
					Level:   "warning",
					Message: sarif.Message{Text: "TODO comment should have an issue reference."},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "pkg/util.go"},
							Region:           sarif.Region{StartLine: 5, EndLine: 5},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence":     0.60,
						"gavel/recommendation": "Add a tracking issue URL to the TODO.",
					},
				},
				{
					RuleID:  "STYLE001",
					Level:   "note",
					Message: sarif.Message{Text: "Consider using a constant for the magic number."},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "pkg/util.go"},
							Region:           sarif.Region{StartLine: 22, EndLine: 22},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence":     0.50,
						"gavel/recommendation": "Extract to a named constant.",
					},
				},
			},
			Properties: map[string]any{
				"gavel/persona": "code-reviewer",
			},
		}},
	}
}

func TestMarkdownFormatter_NilResult(t *testing.T) {
	f := &MarkdownFormatter{}

	t.Run("nil AnalysisOutput", func(t *testing.T) {
		_, err := f.Format(nil)
		if err == nil {
			t.Fatal("expected error for nil AnalysisOutput")
		}
	})

	t.Run("nil Verdict", func(t *testing.T) {
		_, err := f.Format(&AnalysisOutput{
			SARIFLog: testMarkdownLog(),
		})
		if err == nil {
			t.Fatal("expected error for nil Verdict")
		}
	})
}

func TestMarkdownFormatter_HasSummaryHeader(t *testing.T) {
	f := &MarkdownFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)
	if !strings.Contains(output, "## Gavel Analysis Summary") {
		t.Error("output missing '## Gavel Analysis Summary' header")
	}
}

func TestMarkdownFormatter_HasDecisionBanner(t *testing.T) {
	tests := []struct {
		decision string
		contains string
	}{
		{"merge", ":white_check_mark: Merge"},
		{"reject", ":x: Reject"},
		{"review", ":warning: Review Required"},
	}

	for _, tc := range tests {
		t.Run(tc.decision, func(t *testing.T) {
			f := &MarkdownFormatter{}
			result := &AnalysisOutput{
				Verdict:  &store.Verdict{Decision: tc.decision},
				SARIFLog: testMarkdownLog(),
			}
			out, err := f.Format(result)
			if err != nil {
				t.Fatalf("Format() returned error: %v", err)
			}
			output := string(out)
			if !strings.Contains(output, tc.contains) {
				t.Errorf("output missing decision text %q for decision %q", tc.contains, tc.decision)
			}
			// Should also contain findings count.
			if !strings.Contains(output, "**Findings:**") {
				t.Error("output missing findings count in banner")
			}
		})
	}
}

func TestMarkdownFormatter_HasSeverityTable(t *testing.T) {
	f := &MarkdownFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Check table header.
	if !strings.Contains(output, "| Severity") {
		t.Error("output missing severity table header")
	}

	// Check severity rows exist.
	if !strings.Contains(output, "| error") {
		t.Error("output missing error row in severity table")
	}
	if !strings.Contains(output, "| warning") {
		t.Error("output missing warning row in severity table")
	}
	if !strings.Contains(output, "| note") {
		t.Error("output missing note row in severity table")
	}
}

func TestMarkdownFormatter_HasCollapsibleFindings(t *testing.T) {
	f := &MarkdownFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Check for <details> elements.
	if !strings.Contains(output, "<details>") {
		t.Error("output missing <details> collapsible sections")
	}
	if !strings.Contains(output, "</details>") {
		t.Error("output missing </details> closing tags")
	}

	// Check for rule IDs within findings.
	if !strings.Contains(output, "SEC001") {
		t.Error("output missing rule ID SEC001")
	}
	if !strings.Contains(output, "QUAL002") {
		t.Error("output missing rule ID QUAL002")
	}
	if !strings.Contains(output, "STYLE001") {
		t.Error("output missing rule ID STYLE001")
	}

	// Check for confidence display.
	if !strings.Contains(output, "0.95") {
		t.Error("output missing confidence value 0.95")
	}

	// Check for recommendation.
	if !strings.Contains(output, "Use environment variables or a secrets manager.") {
		t.Error("output missing recommendation text")
	}
}

func TestMarkdownFormatter_SortsBySeverity(t *testing.T) {
	f := &MarkdownFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Error findings (SEC001) should appear before warning findings (QUAL002)
	// and warning findings before note findings (STYLE001).
	errorIdx := strings.Index(output, "SEC001")
	warningIdx := strings.Index(output, "QUAL002")
	noteIdx := strings.Index(output, "STYLE001")

	if errorIdx == -1 || warningIdx == -1 || noteIdx == -1 {
		t.Fatalf("missing expected rule IDs in output; error=%d, warning=%d, note=%d", errorIdx, warningIdx, noteIdx)
	}
	if errorIdx >= warningIdx {
		t.Errorf("error finding (pos %d) should appear before warning finding (pos %d)", errorIdx, warningIdx)
	}
	if warningIdx >= noteIdx {
		t.Errorf("warning finding (pos %d) should appear before note finding (pos %d)", warningIdx, noteIdx)
	}
}

func TestMarkdownFormatter_SortsByFilePath(t *testing.T) {
	f := &MarkdownFormatter{}
	// The test data has warnings in internal/handler.go and pkg/util.go.
	// internal/handler.go sorts before pkg/util.go alphabetically.
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Among warnings, QUAL002 (internal/handler.go) should appear before QUAL001 (pkg/util.go).
	qual002Idx := strings.Index(output, "QUAL002")
	qual001Idx := strings.Index(output, "QUAL001")
	if qual002Idx == -1 || qual001Idx == -1 {
		t.Fatalf("missing expected rule IDs; QUAL002=%d, QUAL001=%d", qual002Idx, qual001Idx)
	}
	if qual002Idx >= qual001Idx {
		t.Errorf("QUAL002 (internal/handler.go, pos %d) should appear before QUAL001 (pkg/util.go, pos %d)", qual002Idx, qual001Idx)
	}
}

func TestMarkdownFormatter_HasFooter(t *testing.T) {
	f := &MarkdownFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	if !strings.Contains(output, "Generated by") {
		t.Error("output missing 'Generated by' footer text")
	}
	if !strings.Contains(output, "Gavel") {
		t.Error("output missing 'Gavel' link in footer")
	}
	if !strings.Contains(output, "code-reviewer") {
		t.Error("output missing persona name 'code-reviewer' in footer")
	}
}

func TestMarkdownFormatter_NoFindings(t *testing.T) {
	f := &MarkdownFormatter{}
	log := &sarif.Log{
		Schema:  sarif.SchemaURI,
		Version: sarif.Version,
		Runs: []sarif.Run{{
			Tool: sarif.Tool{
				Driver: sarif.Driver{
					Name:    "gavel",
					Version: "0.1.0",
				},
			},
			Results: []sarif.Result{},
			Properties: map[string]any{
				"gavel/persona": "architect",
			},
		}},
	}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "merge"},
		SARIFLog: log,
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	if !strings.Contains(output, ":white_check_mark: Merge") {
		t.Error("output missing merge decision for no-findings case")
	}
	if !strings.Contains(output, "No findings detected.") {
		t.Error("output missing 'No findings detected.' text")
	}
	if !strings.Contains(output, "architect") {
		t.Error("output missing persona name 'architect' in footer")
	}
	// Should not contain severity table when there are no findings.
	if strings.Contains(output, "### Findings by Severity") {
		t.Error("output should not contain severity table when there are no findings")
	}
}

func TestMarkdownFormatter_NoPersona(t *testing.T) {
	f := &MarkdownFormatter{}
	log := &sarif.Log{
		Schema:  sarif.SchemaURI,
		Version: sarif.Version,
		Runs: []sarif.Run{{
			Tool: sarif.Tool{
				Driver: sarif.Driver{
					Name:    "gavel",
					Version: "0.1.0",
				},
			},
			Results:    []sarif.Result{},
			Properties: map[string]any{},
		}},
	}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "merge"},
		SARIFLog: log,
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Footer should still exist, just without a persona qualifier.
	if !strings.Contains(output, "Generated by") {
		t.Error("output missing footer when persona is absent")
	}
}

func TestMarkdownFormatter_SeverityEmojis(t *testing.T) {
	f := &MarkdownFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	if !strings.Contains(output, ":red_circle:") {
		t.Error("output missing :red_circle: emoji for error severity")
	}
	if !strings.Contains(output, ":warning:") {
		t.Error("output missing :warning: emoji for warning severity")
	}
	if !strings.Contains(output, ":information_source:") {
		t.Error("output missing :information_source: emoji for note severity")
	}
}

func TestMarkdownFormatter_FileLocationInSummary(t *testing.T) {
	f := &MarkdownFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testMarkdownLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Check file count in banner.
	if !strings.Contains(output, "**Files:** 3") {
		t.Errorf("output missing correct file count; expected '**Files:** 3'")
	}

	// Check that findings reference file locations.
	if !strings.Contains(output, "config/db.go") {
		t.Error("output missing file path config/db.go")
	}
	if !strings.Contains(output, "internal/handler.go") {
		t.Error("output missing file path internal/handler.go")
	}
}
