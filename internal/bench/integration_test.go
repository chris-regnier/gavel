package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
)

func TestIntegration_FullBenchmarkPipeline(t *testing.T) {
	if os.Getenv("GAVEL_INTEGRATION") == "" {
		t.Skip("set GAVEL_INTEGRATION=1 to run")
	}

	// Load real corpus
	corpus, err := LoadCorpus("../../benchmarks/corpus")
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("empty corpus")
	}

	// Mock client factory
	factory := func(modelID string) analyzer.BAMLClient {
		return &mockCorpusClient{
			findings: []analyzer.Finding{
				{RuleID: "test-rule", Level: "error", Message: "Test finding for " + modelID, StartLine: 1, EndLine: 1, Confidence: 0.85},
			},
		}
	}

	models := map[string]ModelInfo{
		"fast-model":    {ID: "fast-model", InputPricePerM: 0.5, OutputPricePerM: 3.0},
		"quality-model": {ID: "quality-model", InputPricePerM: 5.0, OutputPricePerM: 25.0},
	}

	cfg := CompareConfig{
		Runs: 2, Parallel: 2,
		Policies: map[string]config.Policy{"shall-be-merged": {Instruction: "check for issues", Enabled: true}},
		Persona:  "code-reviewer",
	}

	report, err := RunComparison(context.Background(), corpus, models, factory, cfg)
	if err != nil {
		t.Fatalf("RunComparison: %v", err)
	}

	if len(report.Models) != 2 {
		t.Errorf("expected 2 model results, got %d", len(report.Models))
	}
	if report.Metadata.CorpusSize != len(corpus.Cases) {
		t.Errorf("corpus size mismatch")
	}

	// Verify JSON round-trip
	var jsonBuf bytes.Buffer
	if err := WriteJSON(&jsonBuf, report); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var parsed ComparisonReport
	if err := json.Unmarshal(jsonBuf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON round-trip failed: %v", err)
	}
	if len(parsed.Models) != 2 {
		t.Errorf("round-trip: expected 2 models, got %d", len(parsed.Models))
	}

	// Verify Markdown output
	var mdBuf bytes.Buffer
	if err := WriteMarkdown(&mdBuf, report); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	if mdBuf.Len() == 0 {
		t.Error("empty markdown output")
	}

	t.Logf("Report: %d models, corpus size %d", len(report.Models), report.Metadata.CorpusSize)
	for _, m := range report.Models {
		t.Logf("  %s: F1=%.2f, P50=%dms, cost=$%.4f/file", m.ModelID, m.Quality.F1, m.Latency.P50Ms, m.Cost.PerFileAvgUSD)
	}
}
