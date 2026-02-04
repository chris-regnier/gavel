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

	e, err := NewEvaluator("")
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

	e, err := NewEvaluator("")
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

	e, err := NewEvaluator("")
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

	e, err := NewEvaluator(dir)
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
