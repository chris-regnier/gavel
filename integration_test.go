package gavel_test

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

type mockClient struct{}

func (m *mockClient) AnalyzeCode(ctx context.Context, code string, policies string, additionalContext string) ([]analyzer.Finding, error) {
	return []analyzer.Finding{
		{
			RuleID:         "error-handling",
			Level:          "warning",
			Message:        "Function Foo ignores error",
			FilePath:       "main.go",
			StartLine:      3,
			EndLine:        3,
			Recommendation: "Handle the error",
			Explanation:    "Error from Bar() is discarded",
			Confidence:     0.8,
		},
	}, nil
}

func TestFullPipeline(t *testing.T) {
	ctx := context.Background()

	// 1. Config
	cfg := config.SystemDefaults()

	// 2. Input — simulate an artifact directly
	artifacts := []input.Artifact{
		{Path: "main.go", Content: "package main\n\nfunc Foo() { Bar() }\n", Kind: input.KindFile},
	}

	// 3. Analyze
	a := analyzer.NewAnalyzer(&mockClient{})
	results, err := a.Analyze(ctx, artifacts, cfg.Policies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// 4. Assemble SARIF
	sarifLog := sarif.Assemble(results, nil, "files")
	if len(sarifLog.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 SARIF result, got %d", len(sarifLog.Runs[0].Results))
	}

	// 5. Store
	dir := t.TempDir()
	fs := store.NewFileStore(dir)
	id, err := fs.WriteSARIF(ctx, sarifLog)
	if err != nil {
		t.Fatal(err)
	}

	// 6. Evaluate
	eval, err := evaluator.NewEvaluator("")
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "review" {
		t.Errorf("expected 'review' for warning-level finding, got %q", verdict.Decision)
	}

	// 7. Store verdict
	if err := fs.WriteVerdict(ctx, id, verdict); err != nil {
		t.Fatal(err)
	}

	// 8. Verify storage round-trip
	loaded, err := fs.ReadVerdict(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Decision != "review" {
		t.Errorf("expected stored verdict 'review', got %q", loaded.Decision)
	}
}
