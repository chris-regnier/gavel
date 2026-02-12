package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
)

// Provider performance test configuration
type providerPerfConfig struct {
	name       string
	provider   config.ProviderConfig
	skipReason string // non-empty to skip
}

// checkOllamaAvailable checks if Ollama is running and returns available models
func checkOllamaAvailable(baseURL string) ([]string, error) {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	// Remove /v1 suffix if present for API call
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	resp, err := http.Get(baseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("ollama not reachable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	models := make([]string, len(result.Models))
	for i, m := range result.Models {
		models[i] = m.Name
	}
	return models, nil
}

// getOllamaTestConfigs returns test configurations for available Ollama models
func getOllamaTestConfigs() []providerPerfConfig {
	models, err := checkOllamaAvailable("")
	if err != nil {
		return []providerPerfConfig{{
			name:       "ollama",
			skipReason: err.Error(),
		}}
	}

	// Preferred models for testing (in order of preference)
	preferredModels := []string{
		"qwen2.5-coder:7b",
		"gpt-oss:20b",
		"deepseek-r1:7b",
		"llama3.2:latest",
		"codellama:latest",
	}

	var configs []providerPerfConfig
	modelSet := make(map[string]bool)
	for _, m := range models {
		modelSet[m] = true
	}

	for _, model := range preferredModels {
		if modelSet[model] {
			configs = append(configs, providerPerfConfig{
				name: fmt.Sprintf("ollama_%s", strings.ReplaceAll(model, ":", "_")),
				provider: config.ProviderConfig{
					Name: "ollama",
					Ollama: config.OllamaConfig{
						Model:   model,
						BaseURL: "http://localhost:11434/v1",
					},
				},
			})
		}
	}

	if len(configs) == 0 {
		return []providerPerfConfig{{
			name:       "ollama",
			skipReason: "no preferred models available",
		}}
	}

	return configs
}

// Test code samples of varying complexity
var testCodeSamples = map[string]string{
	"simple": `package main

func add(a, b int) int {
	return a + b
}
`,
	"medium": `package main

import (
	"fmt"
	"os"
)

func processFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func main() {
	processFile("test.txt")
}
`,
	"complex": `package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type User struct {
	ID        int64     ` + "`json:\"id\"`" + `
	Name      string    ` + "`json:\"name\"`" + `
	Email     string    ` + "`json:\"email\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}

type UserService struct {
	db    *sql.DB
	cache sync.Map
}

func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
	if cached, ok := s.cache.Load(id); ok {
		return cached.(*User), nil
	}

	var user User
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, email, created_at FROM users WHERE id = $1",
		id,
	).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get user %d: %w", id, err)
	}

	s.cache.Store(id, &user)
	return &user, nil
}

func (s *UserService) CreateUser(ctx context.Context, name, email string) (*User, error) {
	var user User
	err := s.db.QueryRowContext(ctx,
		"INSERT INTO users (name, email, created_at) VALUES ($1, $2, $3) RETURNING id, name, email, created_at",
		name, email, time.Now(),
	).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return &user, nil
}

func handleGetUser(svc *UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		var userID int64
		fmt.Sscanf(id, "%d", &userID)

		user, err := svc.GetUser(r.Context(), userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(user)
	}
}
`,
}

// Standard test policies
func getTestPolicies() map[string]config.Policy {
	return map[string]config.Policy{
		"error-handling": {
			Description: "Ensure all errors are properly handled",
			Severity:    "warning",
			Instruction: "Check that all error return values are handled, not ignored",
			Enabled:     true,
		},
		"security": {
			Description: "Check for common security issues",
			Severity:    "error",
			Instruction: "Look for SQL injection, XSS, and other security vulnerabilities",
			Enabled:     true,
		},
	}
}

// LatencyStats holds computed latency statistics
type LatencyStats struct {
	Min    time.Duration
	Max    time.Duration
	Mean   time.Duration
	P50    time.Duration
	P95    time.Duration
	P99    time.Duration
	StdDev time.Duration
}

func computeLatencyStats(latencies []time.Duration) LatencyStats {
	if len(latencies) == 0 {
		return LatencyStats{}
	}

	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, l := range sorted {
		sum += l
	}
	mean := sum / time.Duration(len(sorted))

	var variance float64
	for _, l := range sorted {
		diff := float64(l - mean)
		variance += diff * diff
	}
	variance /= float64(len(sorted))
	stdDev := time.Duration(int64(variance) / int64(time.Microsecond))

	return LatencyStats{
		Min:    sorted[0],
		Max:    sorted[len(sorted)-1],
		Mean:   mean,
		P50:    sorted[len(sorted)*50/100],
		P95:    sorted[len(sorted)*95/100],
		P99:    sorted[len(sorted)*99/100],
		StdDev: stdDev,
	}
}

// TestOllamaProvider_Latency measures response latency for Ollama models
func TestOllamaProvider_Latency(t *testing.T) {
	if os.Getenv("GAVEL_PROVIDER_PERF") == "" {
		t.Skip("set GAVEL_PROVIDER_PERF=1 to run provider performance tests")
	}

	configs := getOllamaTestConfigs()
	policies := getTestPolicies()
	personaPrompt := codeReviewerPrompt

	for _, cfg := range configs {
		if cfg.skipReason != "" {
			t.Skipf("skipping %s: %s", cfg.name, cfg.skipReason)
			continue
		}

		t.Run(cfg.name, func(t *testing.T) {
			client := NewBAMLLiveClient(cfg.provider)
			analyzer := NewAnalyzer(client)

			for sampleName, code := range testCodeSamples {
				t.Run(sampleName, func(t *testing.T) {
					artifacts := []input.Artifact{{
						Path:    "test.go",
						Content: code,
						Kind:    input.KindFile,
					}}

					iterations := 3
					latencies := make([]time.Duration, iterations)

					for i := 0; i < iterations; i++ {
						ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
						start := time.Now()
						results, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
						latencies[i] = time.Since(start)
						cancel()

						if err != nil {
							t.Fatalf("iteration %d failed: %v", i, err)
						}

						t.Logf("Iteration %d: %v, findings: %d", i+1, latencies[i], len(results))
					}

					stats := computeLatencyStats(latencies)
					t.Logf("Latency stats for %s/%s:", cfg.name, sampleName)
					t.Logf("  Min: %v, Max: %v, Mean: %v", stats.Min, stats.Max, stats.Mean)
					t.Logf("  P50: %v, P95: %v, P99: %v", stats.P50, stats.P95, stats.P99)
				})
			}
		})
	}
}

// TestOllamaProvider_Throughput measures requests per minute for Ollama
func TestOllamaProvider_Throughput(t *testing.T) {
	if os.Getenv("GAVEL_PROVIDER_PERF") == "" {
		t.Skip("set GAVEL_PROVIDER_PERF=1 to run provider performance tests")
	}

	configs := getOllamaTestConfigs()
	if len(configs) == 0 || configs[0].skipReason != "" {
		t.Skip("no Ollama models available")
	}

	// Use the first available model (fastest preferred)
	cfg := configs[0]
	policies := getTestPolicies()
	personaPrompt := codeReviewerPrompt

	client := NewBAMLLiveClient(cfg.provider)
	analyzer := NewAnalyzer(client)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: testCodeSamples["simple"],
		Kind:    input.KindFile,
	}}

	// Run for 1 minute or 10 requests, whichever comes first
	duration := 1 * time.Minute
	maxRequests := 10

	var completedRequests int
	var totalLatency time.Duration
	start := time.Now()

	for time.Since(start) < duration && completedRequests < maxRequests {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		reqStart := time.Now()
		_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		reqLatency := time.Since(reqStart)
		cancel()

		if err != nil {
			t.Logf("Request %d failed: %v", completedRequests+1, err)
			continue
		}

		completedRequests++
		totalLatency += reqLatency
		t.Logf("Request %d completed in %v", completedRequests, reqLatency)
	}

	elapsed := time.Since(start)
	throughput := float64(completedRequests) / elapsed.Minutes()
	avgLatency := totalLatency / time.Duration(completedRequests)

	t.Logf("Throughput results for %s:", cfg.name)
	t.Logf("  Completed requests: %d in %v", completedRequests, elapsed)
	t.Logf("  Throughput: %.2f requests/minute", throughput)
	t.Logf("  Average latency: %v", avgLatency)
}

// TestOllamaProvider_Concurrent tests concurrent request handling
func TestOllamaProvider_Concurrent(t *testing.T) {
	if os.Getenv("GAVEL_PROVIDER_PERF") == "" {
		t.Skip("set GAVEL_PROVIDER_PERF=1 to run provider performance tests")
	}

	configs := getOllamaTestConfigs()
	if len(configs) == 0 || configs[0].skipReason != "" {
		t.Skip("no Ollama models available")
	}

	cfg := configs[0]
	policies := getTestPolicies()
	personaPrompt := codeReviewerPrompt

	client := NewBAMLLiveClient(cfg.provider)
	analyzer := NewAnalyzer(client)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: testCodeSamples["simple"],
		Kind:    input.KindFile,
	}}

	concurrencyLevels := []int{1, 2, 3}
	requestsPerWorker := 2

	for _, workers := range concurrencyLevels {
		t.Run(fmt.Sprintf("%d_concurrent", workers), func(t *testing.T) {
			var wg sync.WaitGroup
			var successCount, errorCount atomic.Int64
			var totalLatency atomic.Int64
			latencies := make([]time.Duration, workers*requestsPerWorker)
			var latencyIdx atomic.Int64

			start := time.Now()

			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					for i := 0; i < requestsPerWorker; i++ {
						ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
						reqStart := time.Now()
						_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
						latency := time.Since(reqStart)
						cancel()

						idx := latencyIdx.Add(1) - 1
						if idx < int64(len(latencies)) {
							latencies[idx] = latency
						}
						totalLatency.Add(int64(latency))

						if err != nil {
							errorCount.Add(1)
							t.Logf("Worker %d request %d failed: %v", workerID, i+1, err)
						} else {
							successCount.Add(1)
						}
					}
				}(w)
			}

			wg.Wait()
			elapsed := time.Since(start)

			success := successCount.Load()
			errors := errorCount.Load()
			avgLatency := time.Duration(totalLatency.Load() / (success + errors))

			t.Logf("Concurrent results (%d workers):", workers)
			t.Logf("  Success: %d, Errors: %d", success, errors)
			t.Logf("  Total time: %v", elapsed)
			t.Logf("  Average latency: %v", avgLatency)
			t.Logf("  Effective throughput: %.2f req/min", float64(success)/elapsed.Minutes())

			if errors > 0 && float64(errors)/float64(success+errors) > 0.5 {
				t.Errorf("error rate too high: %d/%d", errors, success+errors)
			}
		})
	}
}

// TestOllamaProvider_ModelComparison compares performance across available models
func TestOllamaProvider_ModelComparison(t *testing.T) {
	if os.Getenv("GAVEL_PROVIDER_PERF") == "" {
		t.Skip("set GAVEL_PROVIDER_PERF=1 to run provider performance tests")
	}

	configs := getOllamaTestConfigs()
	policies := getTestPolicies()
	personaPrompt := codeReviewerPrompt

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: testCodeSamples["medium"],
		Kind:    input.KindFile,
	}}

	type modelResult struct {
		name      string
		latency   time.Duration
		findings  int
		tokenEst  int // rough estimate based on response
		err       error
	}

	var results []modelResult

	for _, cfg := range configs {
		if cfg.skipReason != "" {
			continue
		}

		client := NewBAMLLiveClient(cfg.provider)
		analyzer := NewAnalyzer(client)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		start := time.Now()
		findings, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		latency := time.Since(start)
		cancel()

		results = append(results, modelResult{
			name:     cfg.provider.Ollama.Model,
			latency:  latency,
			findings: len(findings),
			err:      err,
		})
	}

	t.Log("Model Comparison Results:")
	t.Log("=" + strings.Repeat("=", 70))
	t.Logf("%-25s %15s %10s %s", "Model", "Latency", "Findings", "Status")
	t.Log("-" + strings.Repeat("-", 70))

	for _, r := range results {
		status := "OK"
		if r.err != nil {
			status = fmt.Sprintf("ERROR: %v", r.err)
		}
		t.Logf("%-25s %15v %10d %s", r.name, r.latency, r.findings, status)
	}
}

// TestOllamaProvider_CodeSizeScaling tests how latency scales with code size
func TestOllamaProvider_CodeSizeScaling(t *testing.T) {
	if os.Getenv("GAVEL_PROVIDER_PERF") == "" {
		t.Skip("set GAVEL_PROVIDER_PERF=1 to run provider performance tests")
	}

	configs := getOllamaTestConfigs()
	if len(configs) == 0 || configs[0].skipReason != "" {
		t.Skip("no Ollama models available")
	}

	cfg := configs[0]
	policies := getTestPolicies()
	personaPrompt := codeReviewerPrompt

	client := NewBAMLLiveClient(cfg.provider)
	analyzer := NewAnalyzer(client)

	// Generate code of increasing sizes
	sizes := []int{10, 50, 100, 200}

	t.Logf("Code size scaling test for %s:", cfg.provider.Ollama.Model)

	for _, lines := range sizes {
		code := generateCode(lines)
		artifacts := []input.Artifact{{
			Path:    "test.go",
			Content: code,
			Kind:    input.KindFile,
		}}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		start := time.Now()
		results, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		latency := time.Since(start)
		cancel()

		if err != nil {
			t.Logf("  %d lines (%d bytes): ERROR - %v", lines, len(code), err)
			continue
		}

		t.Logf("  %d lines (%d bytes): %v, %d findings", lines, len(code), latency, len(results))
	}
}

// BenchmarkOllamaProvider benchmarks Ollama with real LLM calls
func BenchmarkOllamaProvider(b *testing.B) {
	if os.Getenv("GAVEL_PROVIDER_PERF") == "" {
		b.Skip("set GAVEL_PROVIDER_PERF=1 to run provider benchmarks")
	}

	configs := getOllamaTestConfigs()
	if len(configs) == 0 || configs[0].skipReason != "" {
		b.Skip("no Ollama models available")
	}

	cfg := configs[0]
	policies := getTestPolicies()
	personaPrompt := codeReviewerPrompt

	client := NewBAMLLiveClient(cfg.provider)
	analyzer := NewAnalyzer(client)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: testCodeSamples["simple"],
		Kind:    input.KindFile,
	}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		cancel()
		if err != nil {
			b.Fatalf("benchmark iteration %d failed: %v", i, err)
		}
	}
}
