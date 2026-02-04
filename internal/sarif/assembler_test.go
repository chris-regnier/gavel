package sarif

import (
	"testing"
)

func TestAssemble(t *testing.T) {
	results := []Result{
		{RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue A"}},
		{RuleID: "rule-b", Level: "error", Message: Message{Text: "issue B"}},
	}

	rules := []ReportingDescriptor{
		{ID: "rule-a", ShortDescription: Message{Text: "Rule A"}},
		{ID: "rule-b", ShortDescription: Message{Text: "Rule B"}},
	}

	log := Assemble(results, rules, "diff")

	if len(log.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name != "gavel" {
		t.Errorf("expected tool name 'gavel', got %q", run.Tool.Driver.Name)
	}
	if len(run.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(run.Results))
	}
	if len(run.Tool.Driver.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(run.Tool.Driver.Rules))
	}
	if run.Properties["gavel/inputScope"] != "diff" {
		t.Errorf("expected inputScope 'diff', got %v", run.Properties["gavel/inputScope"])
	}
}

func TestAssemble_Dedup(t *testing.T) {
	results := []Result{
		{
			RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 10, EndLine: 15},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.7},
		},
		{
			RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue duplicate"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 12, EndLine: 18},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.9},
		},
	}

	log := Assemble(results, nil, "files")
	if len(log.Runs[0].Results) != 1 {
		t.Errorf("expected dedup to 1 result, got %d", len(log.Runs[0].Results))
	}
	if log.Runs[0].Results[0].Properties["gavel/confidence"] != 0.9 {
		t.Errorf("expected to keep higher confidence finding")
	}
}
