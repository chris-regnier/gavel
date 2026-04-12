package evaluator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestEvaluator_Reject(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "error-handling",
			Level:   "error",
			Message: sarif.Message{Text: "Critical error"},
			Properties: map[string]interface{}{
				"gavel/confidence": 0.9,
			},
		},
	}

	e, err := NewEvaluator(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "reject" {
		t.Errorf("expected 'reject', got %q", verdict.Decision)
	}
}

func TestEvaluator_Merge(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")

	e, err := NewEvaluator(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "merge" {
		t.Errorf("expected 'merge', got %q", verdict.Decision)
	}
}

func TestEvaluator_Review(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "function-length",
			Level:   "warning",
			Message: sarif.Message{Text: "Function too long"},
			Properties: map[string]interface{}{
				"gavel/confidence": 0.7,
			},
		},
	}

	e, err := NewEvaluator(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "review" {
		t.Errorf("expected 'review', got %q", verdict.Decision)
	}
}

func TestEvaluator_CustomPolicy(t *testing.T) {
	policy := `package gavel.gate

import rego.v1

default decision := "merge"

decision := "reject" if {
    count(input.runs[0].results) > 0
}
`

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "strict.rego"), []byte(policy), 0644)

	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{RuleID: "any-rule", Level: "note", Message: sarif.Message{Text: "Minor"}},
	}

	e, err := NewEvaluator(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "reject" {
		t.Errorf("expected 'reject' from strict policy, got %q", verdict.Decision)
	}
}

// TestEvaluator_BaselineFixedIgnored verifies that a result marked as
// "absent" by baseline comparison (i.e. fixed — it was in the baseline
// but is gone from the current run) does NOT trigger a reject, even
// though it still carries level=error and high confidence.
func TestEvaluator_BaselineFixedIgnored(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:        "sql-injection",
			Level:         "error",
			Message:       sarif.Message{Text: "fixed in this PR"},
			BaselineState: sarif.BaselineStateAbsent,
			Properties:    map[string]interface{}{"gavel/confidence": 0.95},
		},
	}

	e, err := NewEvaluator(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	// With only fixed findings, gating should merge (nothing actionable).
	if verdict.Decision != "merge" {
		t.Errorf("expected 'merge' when only finding is absent/fixed, got %q", verdict.Decision)
	}
}

// TestEvaluator_BaselinePreExistingIgnored verifies that a result marked
// "unchanged" (pre-existing noise) does NOT trigger a reject either.
func TestEvaluator_BaselinePreExistingIgnored(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:        "sql-injection",
			Level:         "error",
			Message:       sarif.Message{Text: "pre-existing"},
			BaselineState: sarif.BaselineStateUnchanged,
			Properties:    map[string]interface{}{"gavel/confidence": 0.95},
		},
	}

	e, err := NewEvaluator(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "merge" {
		t.Errorf("expected 'merge' when only finding is pre-existing, got %q", verdict.Decision)
	}
}

// TestEvaluator_BaselineNewStillRejects verifies that a result marked
// "new" by baseline comparison still triggers a reject when it crosses
// the severity/confidence threshold — this is the regression-gating
// case that baseline tracking is supposed to enable.
func TestEvaluator_BaselineNewStillRejects(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:        "sql-injection",
			Level:         "error",
			Message:       sarif.Message{Text: "regression"},
			BaselineState: sarif.BaselineStateNew,
			Properties:    map[string]interface{}{"gavel/confidence": 0.95},
		},
	}

	e, err := NewEvaluator(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "reject" {
		t.Errorf("expected 'reject' for new high-severity finding, got %q", verdict.Decision)
	}
}

// TestEvaluator_BaselineMixedNewAndPreExisting exercises the whole
// baseline flow: a PR that introduced one new high-severity finding on
// top of an existing one should still reject, and the reject decision
// must key off the new finding, not the pre-existing one.
func TestEvaluator_BaselineMixedNewAndPreExisting(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:        "sql-injection",
			Level:         "error",
			Message:       sarif.Message{Text: "pre-existing"},
			BaselineState: sarif.BaselineStateUnchanged,
			Properties:    map[string]interface{}{"gavel/confidence": 0.95},
		},
		{
			RuleID:        "path-traversal",
			Level:         "error",
			Message:       sarif.Message{Text: "regression"},
			BaselineState: sarif.BaselineStateNew,
			Properties:    map[string]interface{}{"gavel/confidence": 0.9},
		},
	}

	e, err := NewEvaluator(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "reject" {
		t.Errorf("expected 'reject' when PR adds a new regression on top of pre-existing noise, got %q", verdict.Decision)
	}
}
