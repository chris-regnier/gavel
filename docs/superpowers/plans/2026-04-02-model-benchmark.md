# Model Benchmark Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `gavel bench` subcommand that benchmarks multiple OpenRouter models across quality, latency, and cost, producing a spec sheet for model selection.

**Architecture:** New comparison engine (`internal/bench/compare.go`) alongside existing bench harness. Reuses corpus loader and scorer, adds its own orchestration for concurrent multi-model evaluation. CLI wired as `gavel bench` cobra subcommand. Model discovery via OpenRouter API.

**Tech Stack:** Go, Cobra CLI, OpenRouter REST API, existing bench/scorer/corpus packages.

**Spec:** `docs/superpowers/specs/2026-04-02-model-benchmark-design.md`

---

### Task 1: OpenRouter Model Discovery

**Files:**
- Create: `internal/bench/models.go`
- Create: `internal/bench/models_test.go`

- [ ] **Step 1: Write failing test for FetchModels**

```go
// internal/bench/models_test.go
package bench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModels(t *testing.T) {
	resp := OpenRouterModelsResponse{
		Data: []OpenRouterModel{
			{
				ID:   "anthropic/claude-haiku-4.5",
				Name: "Claude Haiku 4.5",
				Pricing: OpenRouterPricing{
					Prompt:     "0.000001",
					Completion: "0.000005",
				},
				ContextLength: 200000,
			},
			{
				ID:   "google/gemini-3-flash-preview",
				Name: "Gemini 3 Flash",
				Pricing: OpenRouterPricing{
					Prompt:     "0.0000005",
					Completion: "0.000003",
				},
				ContextLength: 1048576,
			},
		},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	models, err := FetchModels(context.Background(), "test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "anthropic/claude-haiku-4.5" {
		t.Errorf("expected claude-haiku-4.5, got %s", models[0].ID)
	}
	if models[0].InputPricePerM != 1.0 {
		t.Errorf("expected input price 1.0, got %f", models[0].InputPricePerM)
	}
	if models[0].OutputPricePerM != 5.0 {
		t.Errorf("expected output price 5.0, got %f", models[0].OutputPricePerM)
	}
}

func TestFetchModels_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	_, err := FetchModels(context.Background(), "bad-key", WithBaseURL(srv.URL))
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestFetchModels -v`
Expected: FAIL — `FetchModels` undefined.

- [ ] **Step 3: Implement FetchModels**

```go
// internal/bench/models.go
package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterModelsResponse is the response from GET /api/v1/models.
type OpenRouterModelsResponse struct {
	Data []OpenRouterModel `json:"data"`
}

// OpenRouterModel is a single model from the OpenRouter API.
type OpenRouterModel struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Pricing       OpenRouterPricing  `json:"pricing"`
	ContextLength int                `json:"context_length"`
}

// OpenRouterPricing contains per-token pricing as strings.
type OpenRouterPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// ModelInfo is the parsed, usable model metadata.
type ModelInfo struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	InputPricePerM  float64 `json:"input_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`
	ContextLength   int     `json:"context_length"`
}

type fetchOptions struct {
	baseURL string
}

// FetchOption configures FetchModels behavior.
type FetchOption func(*fetchOptions)

// WithBaseURL overrides the OpenRouter API base URL (for testing).
func WithBaseURL(url string) FetchOption {
	return func(o *fetchOptions) { o.baseURL = url }
}

// FetchModels queries OpenRouter for available programming models.
func FetchModels(ctx context.Context, apiKey string, opts ...FetchOption) ([]ModelInfo, error) {
	o := &fetchOptions{baseURL: defaultOpenRouterBaseURL}
	for _, opt := range opts {
		opt(o)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter API returned %d", resp.StatusCode)
	}

	var result OpenRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		inputPrice, _ := strconv.ParseFloat(m.Pricing.Prompt, 64)
		outputPrice, _ := strconv.ParseFloat(m.Pricing.Completion, 64)
		models = append(models, ModelInfo{
			ID:              m.ID,
			Name:            m.Name,
			InputPricePerM:  inputPrice * 1_000_000,
			OutputPricePerM: outputPrice * 1_000_000,
			ContextLength:   m.ContextLength,
		})
	}
	return models, nil
}

// SortByPrice sorts models by input price ascending.
func SortByPrice(models []ModelInfo) {
	sort.Slice(models, func(i, j int) bool {
		return models[i].InputPricePerM < models[j].InputPricePerM
	})
}

// ValidateModelIDs checks that all requested model IDs are available on OpenRouter.
// Returns a map of model ID -> ModelInfo for valid models, and an error listing any invalid IDs.
func ValidateModelIDs(available []ModelInfo, requested []string) (map[string]ModelInfo, error) {
	lookup := make(map[string]ModelInfo, len(available))
	for _, m := range available {
		lookup[m.ID] = m
	}

	valid := make(map[string]ModelInfo, len(requested))
	var invalid []string
	for _, id := range requested {
		if m, ok := lookup[id]; ok {
			valid[id] = m
		} else {
			invalid = append(invalid, id)
		}
	}

	if len(invalid) > 0 {
		return valid, fmt.Errorf("models not found on OpenRouter: %v", invalid)
	}
	return valid, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run TestFetchModels -v`
Expected: PASS

- [ ] **Step 5: Write test for ValidateModelIDs**

```go
// Add to internal/bench/models_test.go

func TestValidateModelIDs(t *testing.T) {
	available := []ModelInfo{
		{ID: "anthropic/claude-haiku-4.5"},
		{ID: "google/gemini-3-flash-preview"},
		{ID: "deepseek/deepseek-v3.2"},
	}

	t.Run("all valid", func(t *testing.T) {
		valid, err := ValidateModelIDs(available, []string{"anthropic/claude-haiku-4.5", "deepseek/deepseek-v3.2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(valid) != 2 {
			t.Fatalf("expected 2 valid, got %d", len(valid))
		}
	})

	t.Run("some invalid", func(t *testing.T) {
		_, err := ValidateModelIDs(available, []string{"anthropic/claude-haiku-4.5", "openai/gpt-nonexistent"})
		if err == nil {
			t.Fatal("expected error for invalid model")
		}
	})
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/bench/ -run TestValidateModelIDs -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/bench/models.go internal/bench/models_test.go
git commit -m "feat(bench): add OpenRouter model discovery and validation"
```

---

### Task 2: Benchmark Types and BenchClient Wrapper

**Files:**
- Create: `internal/bench/compare.go`
- Create: `internal/bench/compare_test.go`

- [ ] **Step 1: Write failing test for BenchClient timing and token estimation**

```go
// internal/bench/compare_test.go
package bench

import (
	"context"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestBenchClient -v`
Expected: FAIL — `BenchClient` undefined.

- [ ] **Step 3: Implement BenchClient and core types**

```go
// internal/bench/compare.go
package bench

import (
	"context"
	"sync"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
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
	Path           string `json:"path"`
	LatencyMs      int64  `json:"latency_ms"`
	InputTokensEst int    `json:"input_tokens_est"`
	OutputTokensEst int   `json:"output_tokens_est"`
}

// RealWorldModelResult holds real-world benchmark data for a single model.
type RealWorldModelResult struct {
	ModelID string                `json:"model_id"`
	Files   []RealWorldFileResult `json:"files"`
}

// ComparisonMetadata captures experiment configuration.
type ComparisonMetadata struct {
	Timestamp   time.Time `json:"timestamp"`
	RunsPerModel int      `json:"runs_per_model"`
	CorpusSize   int      `json:"corpus_size"`
	CorpusHash   string   `json:"corpus_hash,omitempty"`
	GavelVersion string   `json:"gavel_version,omitempty"`
}

// ComparisonReport is the top-level benchmark output.
type ComparisonReport struct {
	Metadata  ComparisonMetadata     `json:"metadata"`
	Models    []ModelResult          `json:"models"`
	RealWorld []RealWorldModelResult `json:"realworld,omitempty"`
}

// CompareConfig controls comparison engine behavior.
type CompareConfig struct {
	Runs          int
	Parallel      int
	Models        []string
	Policies      map[string]config.Policy
	Persona       string
	CorpusDir     string
	RealWorldDir  string
	OutputDir     string
}
```

Note: the full import block for `compare.go` at this point is:

```go
import (
	"context"
	"sync"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -run TestBenchClient -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bench/compare.go internal/bench/compare_test.go
git commit -m "feat(bench): add benchmark types and BenchClient timing wrapper"
```

---

### Task 3: Latency Aggregation

**Files:**
- Modify: `internal/bench/compare.go`
- Modify: `internal/bench/compare_test.go`

- [ ] **Step 1: Write failing test for latency percentile computation**

```go
// Add to internal/bench/compare_test.go

func TestComputeLatencyMetrics(t *testing.T) {
	calls := []CallRecord{
		{LatencyMs: 100},
		{LatencyMs: 200},
		{LatencyMs: 300},
		{LatencyMs: 400},
		{LatencyMs: 500},
		{LatencyMs: 600},
		{LatencyMs: 700},
		{LatencyMs: 800},
		{LatencyMs: 900},
		{LatencyMs: 1000},
	}
	lm := ComputeLatencyMetrics(calls)

	if lm.MeanMs != 550 {
		t.Errorf("expected mean 550, got %d", lm.MeanMs)
	}
	// P50 = median of 10 values = index 4 (0-based) = 500
	if lm.P50Ms != 500 {
		t.Errorf("expected P50 500, got %d", lm.P50Ms)
	}
	// P95 = index 9 = 1000
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestComputeLatencyMetrics -v`
Expected: FAIL — `ComputeLatencyMetrics` undefined.

- [ ] **Step 3: Implement ComputeLatencyMetrics and ComputeCostMetrics**

```go
// Add to internal/bench/compare.go

import (
	"math"
	"sort"
)

// ComputeLatencyMetrics calculates percentiles from call records.
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

// ComputeCostMetrics calculates cost from call records and model pricing.
func ComputeCostMetrics(calls []CallRecord, model ModelInfo, totalFiles int) CostMetrics {
	var inTok, outTok int64
	for _, c := range calls {
		inTok += int64(c.InputTokensEst)
		outTok += int64(c.OutputTokensEst)
	}
	totalUSD := float64(inTok)/1_000_000*model.InputPricePerM +
		float64(outTok)/1_000_000*model.OutputPricePerM

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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run TestComputeLatencyMetrics -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bench/compare.go internal/bench/compare_test.go
git commit -m "feat(bench): add latency and cost metric computation"
```

---

### Task 4: Comparison Engine Orchestration

**Files:**
- Modify: `internal/bench/compare.go`
- Modify: `internal/bench/compare_test.go`

- [ ] **Step 1: Write failing test for single-model benchmarking**

```go
// Add to internal/bench/compare_test.go

import (
	"github.com/chris-regnier/gavel/internal/config"
)

// mockCorpusClient returns consistent findings for corpus cases.
type mockCorpusClient struct {
	findings []analyzer.Finding
}

func (m *mockCorpusClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
	return m.findings, nil
}

func TestBenchmarkModel(t *testing.T) {
	// Create a minimal in-memory corpus case
	corpus := &Corpus{
		Cases: []Case{
			{
				Name:          "test-case",
				SourceContent: "func main() { password := \"secret\" }",
				ExpectedFindings: []ExpectedFinding{
					{RuleID: "any", Severity: "error", LineRange: [2]int{1, 1}, Category: "security", MustFind: true},
				},
				Metadata: CaseMetadata{Language: "go", Category: "security"},
			},
		},
	}

	client := &mockCorpusClient{
		findings: []analyzer.Finding{
			{RuleID: "hardcoded-secret", Level: "error", Message: "Hardcoded password", StartLine: 1, EndLine: 1, Confidence: 0.9},
		},
	}

	modelInfo := ModelInfo{ID: "test/model", InputPricePerM: 1.0, OutputPricePerM: 5.0}
	policies := map[string]config.Policy{
		"shall-be-merged": {Description: "test", Instruction: "check for issues", Enabled: true},
	}

	result, err := BenchmarkModel(context.Background(), corpus, client, modelInfo, 2, policies, "code-reviewer", 5)
	if err != nil {
		t.Fatalf("BenchmarkModel: %v", err)
	}
	if result.ModelID != "test/model" {
		t.Errorf("expected model test/model, got %s", result.ModelID)
	}
	if result.Quality.F1 == 0 {
		t.Error("expected non-zero F1")
	}
	if len(result.Runs) != 2 {
		t.Errorf("expected 2 run records, got %d", len(result.Runs))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestBenchmarkModel -v`
Expected: FAIL — `BenchmarkModel` undefined.

- [ ] **Step 3: Implement BenchmarkModel**

```go
// Add to internal/bench/compare.go

import (
	"crypto/sha256"
	"fmt"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// BenchmarkModel runs all corpus cases N times against a single model and returns aggregated results.
func BenchmarkModel(ctx context.Context, corpus *Corpus, client analyzer.BAMLClient, model ModelInfo, runs int, policies map[string]config.Policy, persona string, lineTolerance int) (*ModelResult, error) {
	bc := NewBenchClient(client, model)

	policiesText := analyzer.FormatPolicies(policies)
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, persona)
	if err != nil {
		return nil, fmt.Errorf("getting persona prompt: %w", err)
	}

	// Collect per-run scores for variance calculation
	var allCaseScores [][]CaseScore

	for run := 0; run < runs; run++ {
		var runScores []CaseScore
		for _, c := range corpus.Cases {
			findings, err := bc.AnalyzeCode(ctx, c.SourceContent, policiesText, personaPrompt, "")
			if err != nil {
				return nil, fmt.Errorf("model %s, case %s, run %d: %w", model.ID, c.Name, run, err)
			}

			results := findingsToResults(findings, c.Name)
			score := ScoreCase(c, results, lineTolerance)
			runScores = append(runScores, score)
		}
		allCaseScores = append(allCaseScores, runScores)
	}

	// Aggregate quality: average across runs
	quality := aggregateQuality(allCaseScores)

	calls := bc.Calls()
	latency := ComputeLatencyMetrics(calls)
	cost := ComputeCostMetrics(calls, model, len(corpus.Cases)*runs)

	return &ModelResult{
		ModelID: model.ID,
		Quality: quality,
		Latency: latency,
		Cost:    cost,
		Runs:    calls,
	}, nil
}

// findingsToResults converts analyzer findings to SARIF results for scoring.
func findingsToResults(findings []analyzer.Finding, caseName string) []sarif.Result {
	results := make([]sarif.Result, 0, len(findings))
	for _, f := range findings {
		results = append(results, sarif.Result{
			RuleID:  f.RuleID,
			Level:   f.Level,
			Message: sarif.Message{Text: f.Message},
			Locations: []sarif.Location{
				{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: caseName},
						Region:           sarif.Region{StartLine: f.StartLine, EndLine: f.EndLine},
					},
				},
			},
			Properties: map[string]interface{}{
				"gavel/confidence":     f.Confidence,
				"gavel/explanation":    f.Explanation,
				"gavel/recommendation": f.Recommendation,
			},
		})
	}
	return results
}

// aggregateQuality averages CaseScores across multiple runs.
func aggregateQuality(allRuns [][]CaseScore) QualityMetrics {
	if len(allRuns) == 0 {
		return QualityMetrics{}
	}

	var f1s []float64
	var totalTP, totalFP, totalFN, totalHalluc int
	var totalTPConf, totalFPConf float64
	var tpCount, fpCount int

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
			if cs.FalsePositives > 0 {
				totalFPConf += cs.MeanFPConf * float64(cs.FalsePositives)
				fpCount += cs.FalsePositives
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

	// Variance of F1 across runs
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
```

Note: `findingsToResults` in compare.go may duplicate logic from `runner.go`. Check if the existing `findingsToResults` in runner.go is exported — if so, reuse it. If not, keep this local version. The key difference is that this version takes a `caseName` parameter for the artifact URI.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -run TestBenchmarkModel -v`
Expected: PASS

- [ ] **Step 5: Write failing test for RunComparison (concurrent orchestration)**

```go
// Add to internal/bench/compare_test.go

func TestRunComparison(t *testing.T) {
	corpus := &Corpus{
		Cases: []Case{
			{
				Name:          "test-case",
				SourceContent: "x = eval(input())",
				ExpectedFindings: []ExpectedFinding{
					{RuleID: "any", Severity: "error", LineRange: [2]int{1, 1}, Category: "security", MustFind: true},
				},
				Metadata: CaseMetadata{Language: "python", Category: "security"},
			},
		},
	}

	// clientFactory returns a mock client for any model
	clientFactory := func(modelID string) analyzer.BAMLClient {
		return &mockCorpusClient{
			findings: []analyzer.Finding{
				{RuleID: "eval-usage", Level: "error", Message: "Dangerous eval", StartLine: 1, EndLine: 1, Confidence: 0.85},
			},
		}
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
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestRunComparison -v`
Expected: FAIL — `RunComparison` undefined.

- [ ] **Step 7: Implement RunComparison**

```go
// Add to internal/bench/compare.go

import (
	"sync"
)

// ClientFactory creates a BAMLClient for a given model ID.
type ClientFactory func(modelID string) analyzer.BAMLClient

// RunComparison benchmarks multiple models concurrently and returns the full report.
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

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	for res := range resultsCh {
		if res.err != nil {
			return nil, res.err
		}
		report.Models = append(report.Models, *res.result)
	}

	// Sort by F1 descending for consistent output
	sort.Slice(report.Models, func(i, j int) bool {
		return report.Models[i].Quality.F1 > report.Models[j].Quality.F1
	})

	return report, nil
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./internal/bench/ -run TestRunComparison -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/bench/compare.go internal/bench/compare_test.go
git commit -m "feat(bench): add comparison engine with concurrent model evaluation"
```

---

### Task 5: Output Formatting

**Files:**
- Create: `internal/bench/report.go`
- Create: `internal/bench/report_test.go`

- [ ] **Step 1: Write failing test for JSON output**

```go
// internal/bench/report_test.go
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

	// Check for key content
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestWrite -v`
Expected: FAIL — `WriteJSON` undefined.

- [ ] **Step 3: Implement WriteJSON and WriteMarkdown**

```go
// internal/bench/report.go
package bench

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// WriteJSON writes the comparison report as indented JSON.
func WriteJSON(w io.Writer, report *ComparisonReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// WriteMarkdown writes a top-line summary table in Markdown format.
func WriteMarkdown(w io.Writer, report *ComparisonReport) error {
	var sb strings.Builder

	sb.WriteString("# Model Benchmark Results\n\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s | **Runs per model:** %d | **Corpus size:** %d\n\n",
		report.Metadata.Timestamp.Format("2006-01-02"),
		report.Metadata.RunsPerModel,
		report.Metadata.CorpusSize,
	))
	sb.WriteString("## Comparison\n\n")
	sb.WriteString("| Model | F1 | Precision | Recall | Latency (p50) | Cost/File | Use Case |\n")
	sb.WriteString("|-------|-----|-----------|--------|---------------|-----------|----------|\n")

	for _, m := range report.Models {
		useCase := recommendUseCase(m)
		sb.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %.2f | %dms | $%.4f | %s |\n",
			m.ModelID, m.Quality.F1, m.Quality.Precision, m.Quality.Recall,
			m.Latency.P50Ms, m.Cost.PerFileAvgUSD, useCase,
		))
	}

	sb.WriteString("\n## How to reproduce\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString(fmt.Sprintf("gavel bench --runs %d\n", report.Metadata.RunsPerModel))
	sb.WriteString("```\n")
	sb.WriteString("\nFor detailed results, see the structured JSON in `.gavel/bench/`.\n")

	_, err := io.WriteString(w, sb.String())
	return err
}

// recommendUseCase returns a short recommendation based on the model's profile.
func recommendUseCase(m ModelResult) string {
	if m.Quality.F1 >= 0.85 && m.Cost.PerFileAvgUSD > 0.01 {
		return "High-stakes review"
	}
	if m.Quality.F1 >= 0.80 && m.Latency.P50Ms < 3000 {
		return "CI default"
	}
	if m.Cost.PerFileAvgUSD < 0.001 {
		return "Budget / bulk"
	}
	if m.Latency.P50Ms < 2000 {
		return "Fast iteration"
	}
	return "General purpose"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run TestWrite -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bench/report.go internal/bench/report_test.go
git commit -m "feat(bench): add JSON and Markdown report formatting"
```

---

### Task 6: CLI Subcommand

**Files:**
- Create: `cmd/gavel/bench.go`
- Modify: `cmd/gavel/main.go` (add `rootCmd.AddCommand(benchCmd)`)

- [ ] **Step 1: Implement the bench command**

```go
// cmd/gavel/bench.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/bench"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/spf13/cobra"
)

var (
	benchRuns      int
	benchParallel  int
	benchModels    string
	benchCorpusDir string
	benchOutputDir string
	benchFormat    string
)

func init() {
	benchCmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmark models for code analysis quality, latency, and cost",
		RunE:  runBench,
	}
	benchCmd.Flags().IntVar(&benchRuns, "runs", 3, "Number of iterations per model per test case")
	benchCmd.Flags().IntVar(&benchParallel, "parallel", 4, "Max concurrent models")
	benchCmd.Flags().StringVar(&benchModels, "models", "", "Comma-separated OpenRouter model IDs (overrides defaults)")
	benchCmd.Flags().StringVar(&benchCorpusDir, "corpus", "", "Path to corpus directory (default: embedded)")
	benchCmd.Flags().StringVar(&benchOutputDir, "output", ".gavel/bench", "Output directory for results")
	benchCmd.Flags().StringVar(&benchFormat, "format", "json", "Output format: json or yaml")

	modelsCmd := &cobra.Command{
		Use:   "models",
		Short: "List available OpenRouter models with pricing",
		RunE:  runBenchModels,
	}
	var modelsSort string
	var modelsLimit int
	modelsCmd.Flags().StringVar(&modelsSort, "sort", "price", "Sort order: price or name")
	modelsCmd.Flags().IntVar(&modelsLimit, "limit", 20, "Maximum models to display")

	benchCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(benchCmd)
}

func runBench(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable is required")
	}

	// Load config for policies and persona
	cfg, err := config.LoadTiered(
		filepath.Join(os.Getenv("HOME"), ".config", "gavel", "policies.yaml"),
		filepath.Join(".gavel", "policies.yaml"),
	)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Resolve model list
	var modelIDs []string
	if benchModels != "" {
		modelIDs = strings.Split(benchModels, ",")
		for i := range modelIDs {
			modelIDs[i] = strings.TrimSpace(modelIDs[i])
		}
	} else {
		modelIDs = bench.DefaultModels
	}

	// Fetch and validate models from OpenRouter
	slog.Info("fetching available models from OpenRouter")
	available, err := bench.FetchModels(ctx, apiKey)
	if err != nil {
		return fmt.Errorf("fetching models: %w", err)
	}
	modelMap, err := bench.ValidateModelIDs(available, modelIDs)
	if err != nil {
		return fmt.Errorf("validating models: %w", err)
	}
	slog.Info("models validated", "count", len(modelMap))

	// Load corpus
	var corpus *bench.Corpus
	if benchCorpusDir != "" {
		corpus, err = bench.LoadCorpus(benchCorpusDir)
	} else {
		corpus, err = bench.LoadCorpus("benchmarks/corpus")
	}
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	slog.Info("corpus loaded", "cases", len(corpus.Cases))

	// Client factory: creates a BAMLLiveClient configured for each OpenRouter model
	clientFactory := func(modelID string) analyzer.BAMLClient {
		provCfg := config.ProviderConfig{
			Name:       "openrouter",
			OpenRouter: config.OpenRouterConfig{Model: modelID},
		}
		return analyzer.NewBAMLLiveClient(provCfg)
	}

	compareCfg := bench.CompareConfig{
		Runs:     benchRuns,
		Parallel: benchParallel,
		Policies: cfg.Policies,
		Persona:  cfg.Persona,
	}

	slog.Info("starting benchmark", "models", len(modelMap), "runs", benchRuns, "parallel", benchParallel)
	report, err := bench.RunComparison(ctx, corpus, modelMap, clientFactory, compareCfg)
	if err != nil {
		return fmt.Errorf("running comparison: %w", err)
	}

	// Write output
	timestamp := time.Now().Format("20060102-150405")
	outDir := filepath.Join(benchOutputDir, timestamp)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// Structured JSON
	jsonPath := filepath.Join(outDir, "results.json")
	f, err := os.Create(jsonPath)
	if err != nil {
		return fmt.Errorf("creating results file: %w", err)
	}
	if err := bench.WriteJSON(f, report); err != nil {
		f.Close()
		return fmt.Errorf("writing JSON: %w", err)
	}
	f.Close()
	slog.Info("results written", "path", jsonPath)

	// Top-line markdown
	mdPath := filepath.Join("docs", "model-benchmarks.md")
	mf, err := os.Create(mdPath)
	if err != nil {
		return fmt.Errorf("creating markdown file: %w", err)
	}
	if err := bench.WriteMarkdown(mf, report); err != nil {
		mf.Close()
		return fmt.Errorf("writing markdown: %w", err)
	}
	mf.Close()
	slog.Info("summary written", "path", mdPath)

	// Print summary to stdout
	fmt.Printf("\nBenchmark complete: %d models, %d runs each\n", len(report.Models), benchRuns)
	fmt.Printf("Results: %s\n", jsonPath)
	fmt.Printf("Summary: %s\n", mdPath)

	return nil
}

func runBenchModels(cmd *cobra.Command, args []string) error {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable is required")
	}

	models, err := bench.FetchModels(context.Background(), apiKey)
	if err != nil {
		return fmt.Errorf("fetching models: %w", err)
	}

	sortFlag, _ := cmd.Flags().GetString("sort")
	limit, _ := cmd.Flags().GetInt("limit")

	switch sortFlag {
	case "price":
		bench.SortByPrice(models)
	case "name":
		// Already sorted by name from API typically; sort explicitly
		sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	}

	if limit > 0 && len(models) > limit {
		models = models[:limit]
	}

	fmt.Printf("%-45s %10s %10s %10s\n", "MODEL ID", "INPUT $/M", "OUTPUT $/M", "CONTEXT")
	fmt.Printf("%-45s %10s %10s %10s\n", strings.Repeat("-", 45), "----------", "----------", "----------")
	for _, m := range models {
		fmt.Printf("%-45s %10.2f %10.2f %10s\n",
			m.ID, m.InputPricePerM, m.OutputPricePerM, formatContext(m.ContextLength))
	}
	return nil
}

func formatContext(tokens int) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%dM", tokens/1_000_000)
	}
	return fmt.Sprintf("%dK", tokens/1_000)
}
```

Note: add the missing `sort` import at the top of the file.

- [ ] **Step 2: Register the command in main.go**

The `init()` function in `bench.go` handles registration via `rootCmd.AddCommand(benchCmd)`. Verify that `rootCmd` is accessible from `bench.go` — it's defined in `main.go` as a package-level var. Since both files are in `package main`, this works.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/gavel/`
Expected: Compiles successfully.

- [ ] **Step 4: Test help output**

Run: `go run ./cmd/gavel/ bench --help`
Expected: Shows bench command usage with all flags.

Run: `go run ./cmd/gavel/ bench models --help`
Expected: Shows models subcommand usage.

- [ ] **Step 5: Commit**

```bash
git add cmd/gavel/bench.go
git commit -m "feat: add gavel bench CLI subcommand with models discovery"
```

---

### Task 7: Real-World File Set

**Files:**
- Create: `benchmarks/realworld/manifest.yaml`
- Create: `benchmarks/realworld/` — 5-6 source files

This task involves curating real-world code files. The exact files should be sourced at implementation time from popular open-source repositories. Here's the structure to create:

- [ ] **Step 1: Create manifest.yaml**

```yaml
# benchmarks/realworld/manifest.yaml
# Curated real-world files for latency/cost benchmarking.
# These files are NOT scored for quality — only measured for performance.
# Replace with your own files for local experimentation.

files:
  - path: gin-handler.go
    source: github.com/gin-gonic/gin (MIT License)
    description: HTTP handler with middleware, ~200 LOC
    language: go
    lines: 200

  - path: fastapi-crud.py
    source: github.com/tiangolo/fastapi (MIT License)
    description: REST CRUD endpoints with Pydantic models, ~150 LOC
    language: python
    lines: 150

  - path: express-auth.ts
    source: github.com/expressjs/express (MIT License)
    description: Authentication middleware with JWT validation, ~120 LOC
    language: typescript
    lines: 120

  - path: data-pipeline.py
    source: github.com/apache/airflow (Apache 2.0 License)
    description: Data transformation pipeline, ~300 LOC
    language: python
    lines: 300

  - path: cli-parser.go
    source: github.com/spf13/cobra (Apache 2.0 License)
    description: CLI argument parsing and command dispatch, ~250 LOC
    language: go
    lines: 250
```

- [ ] **Step 2: Source and add the real-world files**

For each file in the manifest, extract a representative self-contained snippet from the referenced open-source project. Files should:
- Be self-contained enough for Gavel to analyze meaningfully
- Include the license header / attribution comment at the top
- Cover the target LOC range listed in manifest.yaml
- Include a mix of clean code and realistic issues (not deliberately buggy)

Place each file in `benchmarks/realworld/` matching the `path` field in manifest.yaml.

- [ ] **Step 3: Verify corpus loads**

Run: `ls -la benchmarks/realworld/`
Expected: manifest.yaml plus 5 source files.

- [ ] **Step 4: Commit**

```bash
git add benchmarks/realworld/
git commit -m "feat(bench): add real-world file set for latency/cost benchmarking"
```

---

### Task 8: Wire Real-World Files into Comparison Engine

**Files:**
- Modify: `internal/bench/compare.go`
- Modify: `internal/bench/compare_test.go`

- [ ] **Step 1: Write failing test for real-world file benchmarking**

```go
// Add to internal/bench/compare_test.go

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
	policies := map[string]config.Policy{
		"shall-be-merged": {Instruction: "check", Enabled: true},
	}

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
	if result.Files[0].LatencyMs <= 0 {
		t.Error("expected positive latency")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestBenchmarkRealWorldFiles -v`
Expected: FAIL — `RealWorldFile` and `BenchmarkRealWorldFiles` undefined.

- [ ] **Step 3: Implement RealWorldFile loading and benchmarking**

```go
// Add to internal/bench/compare.go

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

	return &RealWorldModelResult{
		ModelID: model.ID,
		Files:   fileResults,
	}, nil
}
```

Note: add `"gopkg.in/yaml.v3"` and `"os"`, `"path/filepath"` to imports. Check that the project already uses this yaml package (it does — `internal/config/config.go` uses it).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -run TestBenchmarkRealWorldFiles -v`
Expected: PASS

- [ ] **Step 5: Wire real-world files into RunComparison**

Update `RunComparison` to accept an optional `realWorldDir` in `CompareConfig` and run `BenchmarkRealWorldFiles` for each model after the corpus benchmark. Add the results to `report.RealWorld`.

```go
// In RunComparison, after the corpus benchmark loop completes and before returning:

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
```

- [ ] **Step 6: Update bench.go CLI to pass RealWorldDir**

In `cmd/gavel/bench.go`, set `compareCfg.RealWorldDir = "benchmarks/realworld"` (or make it a flag).

- [ ] **Step 7: Run all bench tests**

Run: `go test ./internal/bench/ -v`
Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/bench/compare.go internal/bench/compare_test.go cmd/gavel/bench.go
git commit -m "feat(bench): wire real-world files into comparison engine"
```

---

### Task 9: Integration Test

**Files:**
- Create: `internal/bench/integration_test.go`

- [ ] **Step 1: Write integration test with mock clients**

This test exercises the full pipeline: corpus loading, multi-model comparison, and report generation.

```go
// internal/bench/integration_test.go
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

	// Mock client factory — returns deterministic findings
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
		Runs:     2,
		Parallel: 2,
		Policies: map[string]config.Policy{
			"shall-be-merged": {Instruction: "check for issues", Enabled: true},
		},
		Persona: "code-reviewer",
	}

	report, err := RunComparison(context.Background(), corpus, models, factory, cfg)
	if err != nil {
		t.Fatalf("RunComparison: %v", err)
	}

	// Verify report structure
	if len(report.Models) != 2 {
		t.Errorf("expected 2 model results, got %d", len(report.Models))
	}
	if report.Metadata.CorpusSize != len(corpus.Cases) {
		t.Errorf("corpus size mismatch: %d vs %d", report.Metadata.CorpusSize, len(corpus.Cases))
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
		t.Logf("  %s: F1=%.2f, P50=%dms, cost=$%.4f/file",
			m.ModelID, m.Quality.F1, m.Latency.P50Ms, m.Cost.PerFileAvgUSD)
	}
}
```

- [ ] **Step 2: Run unit tests (no integration flag)**

Run: `go test ./internal/bench/ -v`
Expected: All unit tests pass, integration test skipped.

- [ ] **Step 3: Run integration test**

Run: `GAVEL_INTEGRATION=1 go test ./internal/bench/ -run TestIntegration_FullBenchmarkPipeline -v`
Expected: PASS — full pipeline executes with mock clients against real corpus.

- [ ] **Step 4: Commit**

```bash
git add internal/bench/integration_test.go
git commit -m "test(bench): add integration test for full benchmark pipeline"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass, no regressions.

- [ ] **Step 2: Run lint**

Run: `go vet ./...`
Expected: No issues.

- [ ] **Step 3: Build and verify CLI**

Run: `go build ./cmd/gavel/ && ./gavel bench --help`
Expected: Help output shows all bench flags and subcommands.

Run: `./gavel bench models --help`
Expected: Help output shows models subcommand flags.

- [ ] **Step 4: Final commit if any fixes needed**

```bash
git add -A
git commit -m "chore: final cleanup for model benchmark feature"
```
