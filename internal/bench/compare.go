package bench

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

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

// ComputeLatencyMetrics computes mean and percentile latency from a set of call records.
func ComputeLatencyMetrics(calls []CallRecord) LatencyMetrics {
	if len(calls) == 0 {
		return LatencyMetrics{}
	}
	latencies := make([]int64, len(calls))
	var sum int64
	for i, c := range calls {
		latencies[i] = c.LatencyMs
		sum += c.LatencyMs
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return LatencyMetrics{
		MeanMs: sum / int64(len(latencies)),
		P50Ms:  percentile(latencies, 0.50),
		P95Ms:  percentile(latencies, 0.95),
		P99Ms:  percentile(latencies, 0.99),
	}
}

// percentile returns the value at the given percentile from a sorted int64 slice.
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// ComputeCostMetrics aggregates token counts and computes USD cost from call records.
func ComputeCostMetrics(calls []CallRecord, model ModelInfo, totalFiles int) CostMetrics {
	var inTok, outTok int64
	for _, c := range calls {
		inTok += int64(c.InputTokensEst)
		outTok += int64(c.OutputTokensEst)
	}
	totalUSD := float64(inTok)/1_000_000*model.InputPricePerM + float64(outTok)/1_000_000*model.OutputPricePerM
	var perFile float64
	if totalFiles > 0 {
		perFile = totalUSD / float64(totalFiles)
	}
	return CostMetrics{
		InputTokensTotal:  inTok,
		OutputTokensTotal: outTok,
		InputPricePerM:    model.InputPricePerM,
		OutputPricePerM:   model.OutputPricePerM,
		TotalUSD:          totalUSD,
		PerFileAvgUSD:     perFile,
	}
}

// ClientFactory creates a BAMLClient for a given model ID.
type ClientFactory func(modelID string) analyzer.BAMLClient

// BenchmarkModel runs the corpus against a single model for the given number of
// runs and returns aggregated quality, latency, and cost metrics.
func BenchmarkModel(ctx context.Context, corpus *Corpus, client analyzer.BAMLClient, model ModelInfo, runs int, policies map[string]config.Policy, persona string, lineTolerance int) (*ModelResult, error) {
	bc := NewBenchClient(client, model)
	policiesText := analyzer.FormatPolicies(policies)
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, persona)
	if err != nil {
		return nil, fmt.Errorf("getting persona prompt: %w", err)
	}

	var allCaseScores [][]CaseScore
	for run := 0; run < runs; run++ {
		var runScores []CaseScore
		for _, c := range corpus.Cases {
			findings, err := bc.AnalyzeCode(ctx, c.SourceContent, policiesText, personaPrompt, "")
			if err != nil {
				return nil, fmt.Errorf("model %s, case %s, run %d: %w", model.ID, c.Name, run, err)
			}
			results := findingsToResults(findings)
			score := ScoreCase(c, results, lineTolerance)
			runScores = append(runScores, score)
		}
		allCaseScores = append(allCaseScores, runScores)
	}

	quality := aggregateQuality(allCaseScores)
	calls := bc.Calls()
	return &ModelResult{
		ModelID: model.ID,
		Quality: quality,
		Latency: ComputeLatencyMetrics(calls),
		Cost:    ComputeCostMetrics(calls, model, len(corpus.Cases)*runs),
		Runs:    calls,
	}, nil
}

// RealWorldFile is a file loaded from the real-world benchmark set.
type RealWorldFile struct {
	Path    string
	Content string
}

// LoadRealWorldFiles reads the manifest and source files from a directory.
func LoadRealWorldFiles(dir string) ([]RealWorldFile, error) {
	manifestPath := filepath.Join(dir, "manifest.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest struct {
		Files []struct {
			Path string `yaml:"path"`
		} `yaml:"files"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	var files []RealWorldFile
	for _, entry := range manifest.Files {
		content, err := os.ReadFile(filepath.Join(dir, entry.Path))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Path, err)
		}
		files = append(files, RealWorldFile{Path: entry.Path, Content: string(content)})
	}
	return files, nil
}

// BenchmarkRealWorldFiles runs each file once through a model and captures latency/token data.
func BenchmarkRealWorldFiles(ctx context.Context, files []RealWorldFile, client analyzer.BAMLClient, model ModelInfo, policies map[string]config.Policy, persona string) (*RealWorldModelResult, error) {
	bc := NewBenchClient(client, model)
	policiesText := analyzer.FormatPolicies(policies)
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, persona)
	if err != nil {
		return nil, fmt.Errorf("getting persona prompt: %w", err)
	}

	var fileResults []RealWorldFileResult
	for _, f := range files {
		start := time.Now()
		_, err := bc.AnalyzeCode(ctx, f.Content, policiesText, personaPrompt, "")
		elapsed := time.Since(start)
		if err != nil {
			return nil, fmt.Errorf("analyzing %s: %w", f.Path, err)
		}
		calls := bc.Calls()
		lastCall := calls[len(calls)-1]
		fileResults = append(fileResults, RealWorldFileResult{
			Path:            f.Path,
			LatencyMs:       elapsed.Milliseconds(),
			InputTokensEst:  lastCall.InputTokensEst,
			OutputTokensEst: lastCall.OutputTokensEst,
		})
	}
	return &RealWorldModelResult{ModelID: model.ID, Files: fileResults}, nil
}

// aggregateQuality computes aggregate quality metrics across multiple runs of case scores.
func aggregateQuality(allRuns [][]CaseScore) QualityMetrics {
	if len(allRuns) == 0 {
		return QualityMetrics{}
	}
	var f1s []float64
	var totalTP, totalFP, totalFN, totalHalluc int
	var totalTPConf float64
	var tpCount int

	for _, runScores := range allRuns {
		agg := AggregateScores(runScores)
		f1s = append(f1s, agg.MicroF1)
		totalTP += agg.TotalTP
		totalFP += agg.TotalFP
		totalFN += agg.TotalFN
		totalHalluc += agg.TotalHalluc
		for _, cs := range runScores {
			if cs.TruePositives > 0 {
				totalTPConf += cs.MeanTPConf * float64(cs.TruePositives)
				tpCount += cs.TruePositives
			}
		}
	}

	nRuns := float64(len(allRuns))
	totalResults := float64(totalTP+totalFP) / nRuns
	var precision, recall, f1, hallucinRate, meanConf, variance float64
	if totalTP+totalFP > 0 {
		precision = float64(totalTP) / float64(totalTP+totalFP)
	}
	if totalTP+totalFN > 0 {
		recall = float64(totalTP) / float64(totalTP+totalFN)
	}
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	if totalResults > 0 {
		hallucinRate = float64(totalHalluc) / nRuns / totalResults
	}
	if tpCount > 0 {
		meanConf = totalTPConf / float64(tpCount)
	}
	if len(f1s) > 1 {
		meanF1 := 0.0
		for _, v := range f1s {
			meanF1 += v
		}
		meanF1 /= float64(len(f1s))
		for _, v := range f1s {
			variance += (v - meanF1) * (v - meanF1)
		}
		variance /= float64(len(f1s) - 1)
	}
	return QualityMetrics{
		Precision:         precision,
		Recall:            recall,
		F1:                f1,
		HallucinationRate: hallucinRate,
		MeanConfidence:    meanConf,
		Variance:          variance,
	}
}

// RunComparison benchmarks all provided models concurrently against the corpus
// and returns a ComparisonReport sorted by descending F1 score.
func RunComparison(ctx context.Context, corpus *Corpus, models map[string]ModelInfo, factory ClientFactory, cfg CompareConfig) (*ComparisonReport, error) {
	report := &ComparisonReport{
		Metadata: ComparisonMetadata{
			Timestamp:    time.Now(),
			RunsPerModel: cfg.Runs,
			CorpusSize:   len(corpus.Cases),
		},
	}

	type modelResultOrErr struct {
		result *ModelResult
		err    error
	}
	resultsCh := make(chan modelResultOrErr, len(models))
	sem := make(chan struct{}, cfg.Parallel)
	var wg sync.WaitGroup

	for id, info := range models {
		wg.Add(1)
		go func(modelID string, modelInfo ModelInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			client := factory(modelID)
			result, err := BenchmarkModel(ctx, corpus, client, modelInfo, cfg.Runs, cfg.Policies, cfg.Persona, 5)
			resultsCh <- modelResultOrErr{result: result, err: err}
		}(id, info)
	}

	go func() { wg.Wait(); close(resultsCh) }()

	for res := range resultsCh {
		if res.err != nil {
			return nil, res.err
		}
		report.Models = append(report.Models, *res.result)
	}

	sort.Slice(report.Models, func(i, j int) bool {
		return report.Models[i].Quality.F1 > report.Models[j].Quality.F1
	})

	if cfg.RealWorldDir != "" {
		rwFiles, err := LoadRealWorldFiles(cfg.RealWorldDir)
		if err != nil {
			slog.Warn("skipping real-world files", "error", err)
		} else {
			for id, info := range models {
				client := factory(id)
				rwResult, err := BenchmarkRealWorldFiles(ctx, rwFiles, client, info, cfg.Policies, cfg.Persona)
				if err != nil {
					slog.Warn("real-world benchmark failed", "model", id, "error", err)
					continue
				}
				report.RealWorld = append(report.RealWorld, *rwResult)
			}
		}
	}

	return report, nil
}
