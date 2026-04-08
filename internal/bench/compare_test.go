package bench

import (
	"context"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
)

// mockDelayClient returns findings after a fixed delay.
type mockDelayClient struct {
	delay    time.Duration
	findings []analyzer.Finding
}

func (m *mockDelayClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
	time.Sleep(m.delay)
	return m.findings, nil
}

func TestBenchClient_RecordsTiming(t *testing.T) {
	inner := &mockDelayClient{
		delay: 50 * time.Millisecond,
		findings: []analyzer.Finding{
			{RuleID: "test-rule", Message: "test finding", Level: "warning", Confidence: 0.8},
		},
	}
	bc := NewBenchClient(inner, ModelInfo{
		ID:              "test/model",
		InputPricePerM:  1.0,
		OutputPricePerM: 5.0,
	})

	code := "func main() { fmt.Println(password) }"
	policies := "check for hardcoded secrets"

	findings, err := bc.AnalyzeCode(context.Background(), code, policies, "persona", "")
	if err != nil {
		t.Fatalf("AnalyzeCode: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	calls := bc.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call record, got %d", len(calls))
	}
	call := calls[0]
	if call.LatencyMs < 50 {
		t.Errorf("latency too low: %d ms", call.LatencyMs)
	}
	if call.InputTokensEst <= 0 {
		t.Errorf("expected positive input token estimate, got %d", call.InputTokensEst)
	}
	if call.OutputTokensEst <= 0 {
		t.Errorf("expected positive output token estimate, got %d", call.OutputTokensEst)
	}
}

func TestComputeLatencyMetrics(t *testing.T) {
	calls := []CallRecord{
		{LatencyMs: 100}, {LatencyMs: 200}, {LatencyMs: 300}, {LatencyMs: 400}, {LatencyMs: 500},
		{LatencyMs: 600}, {LatencyMs: 700}, {LatencyMs: 800}, {LatencyMs: 900}, {LatencyMs: 1000},
	}
	lm := ComputeLatencyMetrics(calls)
	if lm.MeanMs != 550 {
		t.Errorf("expected mean 550, got %d", lm.MeanMs)
	}
	if lm.P50Ms != 500 {
		t.Errorf("expected P50 500, got %d", lm.P50Ms)
	}
	if lm.P95Ms != 1000 {
		t.Errorf("expected P95 1000, got %d", lm.P95Ms)
	}
}

func TestComputeLatencyMetrics_Empty(t *testing.T) {
	lm := ComputeLatencyMetrics(nil)
	if lm.MeanMs != 0 {
		t.Errorf("expected 0 mean for empty calls, got %d", lm.MeanMs)
	}
}

// mockCorpusClient is a simple BAMLClient returning fixed findings.
type mockCorpusClient struct {
	findings []analyzer.Finding
}

func (m *mockCorpusClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
	return m.findings, nil
}

func TestBenchmarkModel(t *testing.T) {
	corpus := &Corpus{Cases: []Case{{
		Name:          "test-case",
		SourceContent: "func main() { password := \"secret\" }",
		ExpectedFindings: []ExpectedFinding{
			{RuleID: "any", Severity: "error", LineRange: [2]int{1, 1}, Category: "security", MustFind: true},
		},
		Metadata: CaseMetadata{Language: "go", Category: "security"},
	}}}
	client := &mockCorpusClient{findings: []analyzer.Finding{
		{RuleID: "hardcoded-secret", Level: "error", Message: "Hardcoded password", StartLine: 1, EndLine: 1, Confidence: 0.9},
	}}
	modelInfo := ModelInfo{ID: "test/model", InputPricePerM: 1.0, OutputPricePerM: 5.0}
	policies := map[string]config.Policy{
		"shall-be-merged": {Description: "test", Instruction: "check for issues", Enabled: true},
	}

	result, err := BenchmarkModel(context.Background(), corpus, client, modelInfo, 2, policies, "code-reviewer", 5)
	if err != nil {
		t.Fatalf("BenchmarkModel: %v", err)
	}
	if result.ModelID != "test/model" {
		t.Errorf("expected test/model, got %s", result.ModelID)
	}
	if result.Quality.F1 == 0 {
		t.Error("expected non-zero F1")
	}
	if len(result.Runs) != 2 {
		t.Errorf("expected 2 run records, got %d", len(result.Runs))
	}
}

func TestRunComparison(t *testing.T) {
	corpus := &Corpus{Cases: []Case{{
		Name:          "test-case",
		SourceContent: "x = eval(input())",
		ExpectedFindings: []ExpectedFinding{
			{RuleID: "any", Severity: "error", LineRange: [2]int{1, 1}, Category: "security", MustFind: true},
		},
		Metadata: CaseMetadata{Language: "python", Category: "security"},
	}}}
	clientFactory := func(modelID string) analyzer.BAMLClient {
		return &mockCorpusClient{findings: []analyzer.Finding{
			{RuleID: "eval-usage", Level: "error", Message: "Dangerous eval", StartLine: 1, EndLine: 1, Confidence: 0.85},
		}}
	}
	models := map[string]ModelInfo{
		"model-a": {ID: "model-a", InputPricePerM: 1.0, OutputPricePerM: 5.0},
		"model-b": {ID: "model-b", InputPricePerM: 0.5, OutputPricePerM: 3.0},
	}
	cfg := CompareConfig{
		Runs:     2,
		Parallel: 2,
		Policies: map[string]config.Policy{
			"shall-be-merged": {Instruction: "check for issues", Enabled: true},
		},
		Persona: "code-reviewer",
	}

	report, err := RunComparison(context.Background(), corpus, models, clientFactory, cfg)
	if err != nil {
		t.Fatalf("RunComparison: %v", err)
	}
	if len(report.Models) != 2 {
		t.Fatalf("expected 2 model results, got %d", len(report.Models))
	}
	if report.Metadata.RunsPerModel != 2 {
		t.Errorf("expected 2 runs_per_model, got %d", report.Metadata.RunsPerModel)
	}
}

func TestBenchmarkRealWorldFiles(t *testing.T) {
	files := []RealWorldFile{
		{Path: "test.go", Content: "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}"},
	}
	client := &mockCorpusClient{
		findings: []analyzer.Finding{
			{RuleID: "test", Level: "warning", Message: "test", Confidence: 0.5},
		},
	}
	model := ModelInfo{ID: "test/model", InputPricePerM: 1.0, OutputPricePerM: 5.0}
	policies := map[string]config.Policy{"shall-be-merged": {Instruction: "check", Enabled: true}}

	result, err := BenchmarkRealWorldFiles(context.Background(), files, client, model, policies, "code-reviewer")
	if err != nil {
		t.Fatalf("BenchmarkRealWorldFiles: %v", err)
	}
	if result.ModelID != "test/model" {
		t.Errorf("expected test/model, got %s", result.ModelID)
	}
	if len(result.Files) != 1 {
		t.Errorf("expected 1 file result, got %d", len(result.Files))
	}
	if result.Files[0].LatencyMs < 0 {
		t.Error("expected non-negative latency")
	}
}
