package bench

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func sampleReport() *ComparisonReport {
	return &ComparisonReport{
		Metadata: ComparisonMetadata{
			Timestamp:    time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
			RunsPerModel: 5,
			CorpusSize:   10,
		},
		Models: []ModelResult{
			{
				ModelID: "anthropic/claude-haiku-4.5",
				Quality: QualityMetrics{Precision: 0.85, Recall: 0.90, F1: 0.87},
				Latency: LatencyMetrics{MeanMs: 1200, P50Ms: 1100, P95Ms: 2000, P99Ms: 2500},
				Cost:    CostMetrics{TotalUSD: 0.15, PerFileAvgUSD: 0.003},
			},
			{
				ModelID: "deepseek/deepseek-v3.2",
				Quality: QualityMetrics{Precision: 0.75, Recall: 0.80, F1: 0.77},
				Latency: LatencyMetrics{MeanMs: 800, P50Ms: 700, P95Ms: 1500, P99Ms: 1800},
				Cost:    CostMetrics{TotalUSD: 0.02, PerFileAvgUSD: 0.0004},
			},
		},
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	err := WriteJSON(&buf, sampleReport())
	if err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var parsed ComparisonReport
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(parsed.Models))
	}
}

func TestWriteMarkdown(t *testing.T) {
	var buf bytes.Buffer
	err := WriteMarkdown(&buf, sampleReport())
	if err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("claude-haiku-4.5")) {
		t.Error("markdown missing model name")
	}
	if !bytes.Contains([]byte(output), []byte("|")) {
		t.Error("markdown missing table formatting")
	}
	if !bytes.Contains([]byte(output), []byte("gavel bench")) {
		t.Error("markdown missing re-run instructions")
	}
}
