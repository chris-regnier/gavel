package calibration

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestBuildEventsFromSARIF(t *testing.T) {
	// Build a SARIF log with 1 finding using the actual sarif types from the project.
	log := &sarif.Log{
		Runs: []sarif.Run{{
			Results: []sarif.Result{
				{
					RuleID:  "SEC001",
					Level:   "error",
					Message: sarif.Message{Text: "SQL injection risk"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "main.go"},
							Region:           sarif.Region{StartLine: 10, EndLine: 15},
						},
					}},
					Properties: map[string]interface{}{"gavel/confidence": 0.85},
				},
			},
		}},
	}

	events := BuildEventsFromSARIF(log, "result-123", "code-reviewer", "openrouter", "claude-sonnet", false)
	if len(events) < 2 {
		t.Fatalf("events = %d, want >= 2", len(events))
	}
	if events[0].Type != EventAnalysisCompleted {
		t.Errorf("first type = %q, want analysis_completed", events[0].Type)
	}
	if events[1].Type != EventFindingCreated {
		t.Errorf("second type = %q, want finding_created", events[1].Type)
	}

	// Verify AnalysisPayload fields.
	ap, ok := events[0].Payload.(AnalysisPayload)
	if !ok {
		t.Fatalf("first event payload is %T, want AnalysisPayload", events[0].Payload)
	}
	if ap.ResultID != "result-123" {
		t.Errorf("AnalysisPayload.ResultID = %q, want %q", ap.ResultID, "result-123")
	}
	if ap.FindingCount != 1 {
		t.Errorf("AnalysisPayload.FindingCount = %d, want 1", ap.FindingCount)
	}
	if ap.Provider != "openrouter" {
		t.Errorf("AnalysisPayload.Provider = %q, want %q", ap.Provider, "openrouter")
	}
	if ap.Model != "claude-sonnet" {
		t.Errorf("AnalysisPayload.Model = %q, want %q", ap.Model, "claude-sonnet")
	}
	if ap.Persona != "code-reviewer" {
		t.Errorf("AnalysisPayload.Persona = %q, want %q", ap.Persona, "code-reviewer")
	}
	if len(ap.RuleIDs) != 1 || ap.RuleIDs[0] != "SEC001" {
		t.Errorf("AnalysisPayload.RuleIDs = %v, want [SEC001]", ap.RuleIDs)
	}
	if len(ap.FileTypes) != 1 || ap.FileTypes[0] != ".go" {
		t.Errorf("AnalysisPayload.FileTypes = %v, want [.go]", ap.FileTypes)
	}

	// Verify FindingPayload fields.
	fp, ok := events[1].Payload.(FindingPayload)
	if !ok {
		t.Fatalf("second event payload is %T, want FindingPayload", events[1].Payload)
	}
	if fp.ResultID != "result-123" {
		t.Errorf("FindingPayload.ResultID = %q, want %q", fp.ResultID, "result-123")
	}
	if fp.RuleID != "SEC001" {
		t.Errorf("FindingPayload.RuleID = %q, want %q", fp.RuleID, "SEC001")
	}
	if fp.Severity != "error" {
		t.Errorf("FindingPayload.Severity = %q, want %q", fp.Severity, "error")
	}
	if fp.Confidence != 0.85 {
		t.Errorf("FindingPayload.Confidence = %f, want 0.85", fp.Confidence)
	}
	if fp.FileType != ".go" {
		t.Errorf("FindingPayload.FileType = %q, want %q", fp.FileType, ".go")
	}
	if fp.StartLine != 10 {
		t.Errorf("FindingPayload.StartLine = %d, want 10", fp.StartLine)
	}
	if fp.EndLine != 15 {
		t.Errorf("FindingPayload.EndLine = %d, want 15", fp.EndLine)
	}
	if fp.Message != "SQL injection risk" {
		t.Errorf("FindingPayload.Message = %q, want %q", fp.Message, "SQL injection risk")
	}
}

func TestBuildEventsFromSARIF_EmptyRuns(t *testing.T) {
	log := &sarif.Log{}
	events := BuildEventsFromSARIF(log, "id", "", "", "", false)
	if events != nil {
		t.Errorf("expected nil for empty runs, got %d events", len(events))
	}
}

func TestBuildEventsFromSARIF_MultipleFindings(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{{
			Results: []sarif.Result{
				{
					RuleID:  "SEC001",
					Level:   "error",
					Message: sarif.Message{Text: "finding one"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "foo.py"},
							Region:           sarif.Region{StartLine: 1, EndLine: 2},
						},
					}},
				},
				{
					RuleID:  "SEC002",
					Level:   "warning",
					Message: sarif.Message{Text: "finding two"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "bar.go"},
							Region:           sarif.Region{StartLine: 5, EndLine: 6},
						},
					}},
				},
			},
		}},
	}

	events := BuildEventsFromSARIF(log, "result-456", "security", "anthropic", "claude-opus", false)
	// Expect 1 analysis_completed + 2 finding_created = 3 events total.
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}

	ap, ok := events[0].Payload.(AnalysisPayload)
	if !ok {
		t.Fatalf("first event payload is %T, want AnalysisPayload", events[0].Payload)
	}
	if ap.FindingCount != 2 {
		t.Errorf("AnalysisPayload.FindingCount = %d, want 2", ap.FindingCount)
	}
	if len(ap.RuleIDs) != 2 {
		t.Errorf("AnalysisPayload.RuleIDs len = %d, want 2", len(ap.RuleIDs))
	}
	// Both .py and .go file types should be collected.
	if len(ap.FileTypes) != 2 {
		t.Errorf("AnalysisPayload.FileTypes len = %d, want 2", len(ap.FileTypes))
	}
}

func TestBuildEventsFromSARIF_NoConfidence(t *testing.T) {
	// A finding with no gavel/confidence property should default to 0.
	log := &sarif.Log{
		Runs: []sarif.Run{{
			Results: []sarif.Result{
				{
					RuleID:  "QA001",
					Level:   "note",
					Message: sarif.Message{Text: "style issue"},
				},
			},
		}},
	}

	events := BuildEventsFromSARIF(log, "result-789", "code-reviewer", "ollama", "qwen2.5", false)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	fp, ok := events[1].Payload.(FindingPayload)
	if !ok {
		t.Fatalf("second event payload is %T, want FindingPayload", events[1].Payload)
	}
	if fp.Confidence != 0 {
		t.Errorf("FindingPayload.Confidence = %f, want 0 when property absent", fp.Confidence)
	}
	if fp.FileType != "" {
		t.Errorf("FindingPayload.FileType = %q, want empty when no locations", fp.FileType)
	}
}
