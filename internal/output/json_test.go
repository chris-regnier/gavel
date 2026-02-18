package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/store"
)

func TestJSONFormatter_Format(t *testing.T) {
	f := &JSONFormatter{}
	result := &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "merge",
			Reason:   "no issues found",
		},
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("JSONFormatter.Format() returned error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("JSONFormatter.Format() returned empty output")
	}

	// Verify it is valid JSON (strip trailing newline for unmarshal).
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}

	// Verify the decision field is present and correct.
	decision, ok := parsed["decision"]
	if !ok {
		t.Fatal("parsed JSON missing 'decision' field")
	}
	if decision != "merge" {
		t.Errorf("decision = %q, want %q", decision, "merge")
	}

	// Verify the reason field is present and correct.
	reason, ok := parsed["reason"]
	if !ok {
		t.Fatal("parsed JSON missing 'reason' field")
	}
	if reason != "no issues found" {
		t.Errorf("reason = %q, want %q", reason, "no issues found")
	}
}

func TestJSONFormatter_NilVerdict(t *testing.T) {
	f := &JSONFormatter{}

	t.Run("nil AnalysisOutput", func(t *testing.T) {
		_, err := f.Format(nil)
		if err == nil {
			t.Fatal("expected error for nil AnalysisOutput")
		}
		if !strings.Contains(err.Error(), "verdict is required") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "verdict is required")
		}
	})

	t.Run("nil Verdict field", func(t *testing.T) {
		_, err := f.Format(&AnalysisOutput{})
		if err == nil {
			t.Fatal("expected error for nil verdict")
		}
		if !strings.Contains(err.Error(), "verdict is required") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "verdict is required")
		}
	})
}

func TestJSONFormatter_TrailingNewline(t *testing.T) {
	f := &JSONFormatter{}
	result := &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "review",
			Reason:   "findings require human review",
		},
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("JSONFormatter.Format() returned error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("JSONFormatter.Format() returned empty output")
	}
	if out[len(out)-1] != '\n' {
		t.Errorf("output does not end with trailing newline; last byte = %q", out[len(out)-1])
	}
}

func TestJSONFormatter_OmitsEmptyOptionalFields(t *testing.T) {
	f := &JSONFormatter{}
	result := &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "merge",
			Reason:   "clean",
			// RelevantFindings and Metadata are nil — should be omitted.
		},
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("JSONFormatter.Format() returned error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if _, ok := parsed["relevant_findings"]; ok {
		t.Error("expected 'relevant_findings' to be omitted when empty")
	}
	if _, ok := parsed["metadata"]; ok {
		t.Error("expected 'metadata' to be omitted when empty")
	}
}

func TestJSONFormatter_WithMetadata(t *testing.T) {
	f := &JSONFormatter{}
	result := &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "reject",
			Reason:   "critical security issue",
			Metadata: map[string]interface{}{
				"provider": "anthropic",
				"model":    "claude-sonnet-4",
			},
		},
	}
	out, err := f.Format(result)
	if err != nil {
		t.Fatalf("JSONFormatter.Format() returned error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	meta, ok := parsed["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'metadata' to be present as a map")
	}
	if meta["provider"] != "anthropic" {
		t.Errorf("metadata.provider = %q, want %q", meta["provider"], "anthropic")
	}
	if meta["model"] != "claude-sonnet-4" {
		t.Errorf("metadata.model = %q, want %q", meta["model"], "claude-sonnet-4")
	}
}
