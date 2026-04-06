package bench

import (
	"context"
	"sync"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
)

// DefaultModels is the default set of OpenRouter models to benchmark.
var DefaultModels = []string{
	"anthropic/claude-opus-4.6",
	"openai/gpt-5.3-codex",
	"google/gemini-3.1-pro-preview",
	"anthropic/claude-sonnet-4.6",
	"anthropic/claude-haiku-4.5",
	"google/gemini-3-flash-preview",
	"deepseek/deepseek-v3.2",
	"qwen/qwen3-coder-next",
}

// CallRecord captures timing and token data for a single AnalyzeCode call.
type CallRecord struct {
	LatencyMs       int64 `json:"latency_ms"`
	InputTokensEst  int   `json:"input_tokens_est"`
	OutputTokensEst int   `json:"output_tokens_est"`
}

// BenchClient wraps a BAMLClient to record timing and estimate tokens per call.
type BenchClient struct {
	inner analyzer.BAMLClient
	model ModelInfo
	mu    sync.Mutex
	calls []CallRecord
}

// NewBenchClient wraps a BAMLClient for benchmarking.
func NewBenchClient(inner analyzer.BAMLClient, model ModelInfo) *BenchClient {
	return &BenchClient{inner: inner, model: model}
}

// AnalyzeCode delegates to the inner client and records timing/token estimates.
func (bc *BenchClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
	start := time.Now()
	findings, err := bc.inner.AnalyzeCode(ctx, code, policies, persona, additional)
	elapsed := time.Since(start)

	if err == nil {
		inputChars := len(code) + len(policies) + len(persona) + len(additional)
		outputChars := 0
		for _, f := range findings {
			outputChars += len(f.Message) + len(f.Explanation) + len(f.Recommendation) + len(f.RuleID) + len(f.Level)
		}
		record := CallRecord{
			LatencyMs:       elapsed.Milliseconds(),
			InputTokensEst:  inputChars / 4,
			OutputTokensEst: outputChars / 4,
		}
		bc.mu.Lock()
		bc.calls = append(bc.calls, record)
		bc.mu.Unlock()
	}
	return findings, err
}

// Calls returns all recorded call data.
func (bc *BenchClient) Calls() []CallRecord {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	out := make([]CallRecord, len(bc.calls))
	copy(out, bc.calls)
	return out
}

// QualityMetrics holds precision/recall/F1 scores for a model.
type QualityMetrics struct {
	Precision         float64 `json:"precision"`
	Recall            float64 `json:"recall"`
	F1                float64 `json:"f1"`
	HallucinationRate float64 `json:"hallucination_rate"`
	MeanConfidence    float64 `json:"mean_confidence"`
	Variance          float64 `json:"variance"`
}

// LatencyMetrics holds latency percentiles for a model.
type LatencyMetrics struct {
	MeanMs int64 `json:"mean_ms"`
	P50Ms  int64 `json:"p50_ms"`
	P95Ms  int64 `json:"p95_ms"`
	P99Ms  int64 `json:"p99_ms"`
}

// CostMetrics holds token and cost data for a model.
type CostMetrics struct {
	InputTokensTotal  int64   `json:"input_tokens_total"`
	OutputTokensTotal int64   `json:"output_tokens_total"`
	InputPricePerM    float64 `json:"input_price_per_m"`
	OutputPricePerM   float64 `json:"output_price_per_m"`
	TotalUSD          float64 `json:"total_usd"`
	PerFileAvgUSD     float64 `json:"per_file_avg_usd"`
}

// ModelResult holds all benchmark data for a single model.
type ModelResult struct {
	ModelID string         `json:"model_id"`
	Tier    string         `json:"tier,omitempty"`
	Quality QualityMetrics `json:"quality"`
	Latency LatencyMetrics `json:"latency"`
	Cost    CostMetrics    `json:"cost"`
	Runs    []CallRecord   `json:"runs"`
}

// RealWorldFileResult holds latency/cost data for a single file on a single model.
type RealWorldFileResult struct {
	Path            string `json:"path"`
	LatencyMs       int64  `json:"latency_ms"`
	InputTokensEst  int    `json:"input_tokens_est"`
	OutputTokensEst int    `json:"output_tokens_est"`
}

// RealWorldModelResult holds real-world benchmark data for a single model.
type RealWorldModelResult struct {
	ModelID string                `json:"model_id"`
	Files   []RealWorldFileResult `json:"files"`
}

// ComparisonMetadata captures experiment configuration.
type ComparisonMetadata struct {
	Timestamp    time.Time `json:"timestamp"`
	RunsPerModel int       `json:"runs_per_model"`
	CorpusSize   int       `json:"corpus_size"`
	CorpusHash   string    `json:"corpus_hash,omitempty"`
	GavelVersion string    `json:"gavel_version,omitempty"`
}

// ComparisonReport is the top-level benchmark output.
type ComparisonReport struct {
	Metadata  ComparisonMetadata     `json:"metadata"`
	Models    []ModelResult          `json:"models"`
	RealWorld []RealWorldModelResult `json:"realworld,omitempty"`
}

// CompareConfig controls comparison engine behavior.
type CompareConfig struct {
	Runs         int
	Parallel     int
	Models       []string
	Policies     map[string]config.Policy
	Persona      string
	CorpusDir    string
	RealWorldDir string
	OutputDir    string
}
