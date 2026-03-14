package bench

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
)

type mockBenchClient struct{}

func (m *mockBenchClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
	return []analyzer.Finding{
		{RuleID: "SEC001", Level: "error", Message: "SQL injection", StartLine: 12, EndLine: 14, Confidence: 0.9},
	}, nil
}

func TestRunBenchmark(t *testing.T) {
	corpus := &Corpus{
		Cases: []Case{
			{
				Name:          "sql-injection",
				SourcePath:    "source.go",
				SourceContent: "package main\nfunc query() {}",
				ExpectedFindings: []ExpectedFinding{
					{RuleID: "SEC001", Severity: "error", LineRange: [2]int{10, 15}, MustFind: true},
				},
			},
		},
	}

	cfg := RunConfig{
		Runs:          2,
		LineTolerance: 5,
		Policies:      config.SystemDefaults().Policies,
		Persona:       "code-reviewer",
	}

	result, err := RunBenchmark(context.Background(), corpus, &mockBenchClient{}, cfg)
	if err != nil {
		t.Fatalf("RunBenchmark: %v", err)
	}
	if result.Runs != 2 {
		t.Errorf("Runs = %d, want 2", result.Runs)
	}
	if len(result.PerCase) != 1 {
		t.Fatalf("PerCase = %d, want 1", len(result.PerCase))
	}
	if result.Aggregate.MicroRecall == 0 {
		t.Error("MicroRecall should be > 0")
	}
}
