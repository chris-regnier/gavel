package sarif

import (
	"encoding/json"
	"testing"
)

func TestSarifLog_MarshalJSON(t *testing.T) {
	log := NewLog("gavel", "0.1.0")
	log.Runs[0].Results = append(log.Runs[0].Results, Result{
		RuleID:  "error-handling",
		Level:   "warning",
		Message: Message{Text: "Function Foo does not handle errors"},
		Locations: []Location{{
			PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "pkg/bar/bar.go"},
				Region:           Region{StartLine: 10, EndLine: 15},
			},
		}},
		Properties: map[string]interface{}{
			"gavel/recommendation": "Add error return",
			"gavel/explanation":    "Function calls DB but ignores error",
			"gavel/confidence":     0.9,
		},
	})

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var parsed Log
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if len(parsed.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(parsed.Runs))
	}
	if len(parsed.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(parsed.Runs[0].Results))
	}
	r := parsed.Runs[0].Results[0]
	if r.RuleID != "error-handling" {
		t.Errorf("expected ruleId 'error-handling', got %q", r.RuleID)
	}
	if r.Properties["gavel/recommendation"] != "Add error return" {
		t.Errorf("expected recommendation preserved")
	}
}
