package output

import (
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// testPrettyLog builds a SARIF log with multiple findings across files and
// severities, suitable for exercising the pretty formatter.
func testPrettyLog() *sarif.Log {
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
					Message: sarif.Message{Text: "Hardcoded secret detected"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
							Region:           sarif.Region{StartLine: 42, EndLine: 42},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence": 0.95,
					},
				},
				{
					RuleID:  "ERR003",
					Level:   "warning",
					Message: sarif.Message{Text: "Error not checked"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
							Region:           sarif.Region{StartLine: 78, EndLine: 78},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence": 0.82,
					},
				},
				{
					RuleID:  "SEC005",
					Level:   "warning",
					Message: sarif.Message{Text: "SQL string concatenation"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "internal/handler.go"},
							Region:           sarif.Region{StartLine: 15, EndLine: 15},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence": 0.88,
					},
				},
				{
					RuleID:  "STY001",
					Level:   "note",
					Message: sarif.Message{Text: "Function exceeds 50 lines"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "internal/handler.go"},
							Region:           sarif.Region{StartLine: 23, EndLine: 23},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence": 0.70,
					},
				},
				{
					RuleID:  "ERR001",
					Level:   "warning",
					Message: sarif.Message{Text: "Panic in init function"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "cmd/main.go"},
							Region:           sarif.Region{StartLine: 9, EndLine: 9},
						},
					}},
					Properties: map[string]any{
						"gavel/confidence": 0.75,
					},
				},
			},
			Properties: map[string]any{
				"gavel/persona": "code-reviewer",
			},
		}},
	}
}

func TestPrettyFormatter_ContainsDecision(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testPrettyLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)
	if !strings.Contains(output, "review") {
		t.Error("output missing decision string 'review'")
	}
}

func TestPrettyFormatter_GroupsByFile(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testPrettyLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// All three file paths should appear.
	for _, path := range []string{"cmd/main.go", "config/db.go", "internal/handler.go"} {
		if !strings.Contains(output, path) {
			t.Errorf("output missing file path %q", path)
		}
	}

	// Files should be sorted alphabetically: cmd/main.go < config/db.go < internal/handler.go.
	cmdIdx := strings.Index(output, "cmd/main.go")
	configIdx := strings.Index(output, "config/db.go")
	internalIdx := strings.Index(output, "internal/handler.go")
	if cmdIdx >= configIdx {
		t.Errorf("cmd/main.go (pos %d) should appear before config/db.go (pos %d)", cmdIdx, configIdx)
	}
	if configIdx >= internalIdx {
		t.Errorf("config/db.go (pos %d) should appear before internal/handler.go (pos %d)", configIdx, internalIdx)
	}
}

func TestPrettyFormatter_ContainsRuleIDs(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testPrettyLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	for _, ruleID := range []string{"SEC001", "ERR003", "SEC005", "STY001", "ERR001"} {
		if !strings.Contains(output, ruleID) {
			t.Errorf("output missing rule ID %q", ruleID)
		}
	}
}

func TestPrettyFormatter_HasSummaryLine(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testPrettyLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// 1 error (singular), 3 warnings (plural), 1 note (singular).
	if !strings.Contains(output, "1 error") {
		t.Error("output missing '1 error' in summary")
	}
	if !strings.Contains(output, "3 warnings") {
		t.Error("output missing '3 warnings' in summary")
	}
	if !strings.Contains(output, "1 note") {
		t.Error("output missing '1 note' in summary")
	}
}

func TestPrettyFormatter_NoFindings(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
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

	if !strings.Contains(output, "No findings") {
		t.Error("output missing 'No findings' text for empty results")
	}
}

func TestPrettyFormatter_NilResult(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	_, err := f.Format(nil)
	if err == nil {
		t.Fatal("expected error for nil AnalysisOutput")
	}
}

func TestPrettyFormatter_SortsWithinFile(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testPrettyLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Within config/db.go: SEC001 (line 42) should appear before ERR003 (line 78).
	sec001Idx := strings.Index(output, "SEC001")
	err003Idx := strings.Index(output, "ERR003")
	if sec001Idx == -1 || err003Idx == -1 {
		t.Fatalf("missing expected rule IDs; SEC001=%d, ERR003=%d", sec001Idx, err003Idx)
	}
	if sec001Idx >= err003Idx {
		t.Errorf("SEC001 (line 42, pos %d) should appear before ERR003 (line 78, pos %d)", sec001Idx, err003Idx)
	}

	// Within internal/handler.go: SEC005 (line 15) should appear before STY001 (line 23).
	sec005Idx := strings.Index(output, "SEC005")
	sty001Idx := strings.Index(output, "STY001")
	if sec005Idx == -1 || sty001Idx == -1 {
		t.Fatalf("missing expected rule IDs; SEC005=%d, STY001=%d", sec005Idx, sty001Idx)
	}
	if sec005Idx >= sty001Idx {
		t.Errorf("SEC005 (line 15, pos %d) should appear before STY001 (line 23, pos %d)", sec005Idx, sty001Idx)
	}
}

func TestPrettyFormatter_ContainsConfidence(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testPrettyLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Check that confidence values appear in the output.
	if !strings.Contains(output, "0.95") {
		t.Error("output missing confidence value 0.95")
	}
	if !strings.Contains(output, "0.82") {
		t.Error("output missing confidence value 0.82")
	}
}

func TestPrettyFormatter_ContainsPersona(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "review"},
		SARIFLog: testPrettyLog(),
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	if !strings.Contains(output, "code-reviewer") {
		t.Error("output missing persona 'code-reviewer'")
	}
}

func TestPrettyFormatter_NilSARIFLog(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	f := &PrettyFormatter{}
	result := &AnalysisOutput{
		Verdict:  &store.Verdict{Decision: "merge"},
		SARIFLog: nil,
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("Format() returned error: %v", err)
	}
	output := string(out)

	// Should still produce output with "No findings" when SARIF is nil.
	if !strings.Contains(output, "No findings") {
		t.Error("output missing 'No findings' text when SARIFLog is nil")
	}
	if !strings.Contains(output, "merge") {
		t.Error("output missing decision 'merge' when SARIFLog is nil")
	}
}
