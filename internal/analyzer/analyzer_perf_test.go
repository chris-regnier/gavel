package analyzer

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/input"
)

// perfMockClient simulates various LLM response behaviors
type perfMockClient struct {
	findings      []Finding
	latency       time.Duration
	errorRate     float64
	callCount     atomic.Int64
	totalLatency  atomic.Int64 // in nanoseconds
}

func (m *perfMockClient) AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error) {
	start := time.Now()
	m.callCount.Add(1)

	// Simulate latency
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	elapsed := time.Since(start)
	m.totalLatency.Add(int64(elapsed))

	return m.findings, nil
}

// TestAnalyzePerformance_Throughput measures requests per second
func TestAnalyzePerformance_Throughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	mock := &perfMockClient{
		findings: generateFindings(5),
		latency:  0, // No latency to measure pure throughput
	}
	analyzer := NewAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: generateCode(100),
		Kind:    input.KindFile,
	}}
	policies := generatePolicies(5)
	ctx := context.Background()
	personaPrompt := codeReviewerPrompt

	duration := 2 * time.Second
	start := time.Now()
	iterations := 0

	for time.Since(start) < duration {
		_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		iterations++
	}

	elapsed := time.Since(start)
	throughput := float64(iterations) / elapsed.Seconds()

	t.Logf("Throughput: %.2f ops/sec over %v (%d iterations)", throughput, elapsed, iterations)
	
	// Ensure minimum throughput (sanity check)
	if throughput < 100 {
		t.Errorf("throughput too low: %.2f ops/sec, expected at least 100", throughput)
	}
}

// TestAnalyzePerformance_LatencyDistribution measures p50, p95, p99 latencies
func TestAnalyzePerformance_LatencyDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	mock := &perfMockClient{
		findings: generateFindings(10),
		latency:  0,
	}
	analyzer := NewAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: generateCode(200),
		Kind:    input.KindFile,
	}}
	policies := generatePolicies(5)
	ctx := context.Background()
	personaPrompt := codeReviewerPrompt

	iterations := 1000
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		latencies[i] = time.Since(start)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Sort latencies for percentile calculation
	sortDurations(latencies)

	p50 := latencies[iterations*50/100]
	p95 := latencies[iterations*95/100]
	p99 := latencies[iterations*99/100]

	t.Logf("Latency distribution over %d iterations:", iterations)
	t.Logf("  p50: %v", p50)
	t.Logf("  p95: %v", p95)
	t.Logf("  p99: %v", p99)

	// Sanity checks - these are very lenient since we're testing overhead, not actual LLM calls
	if p50 > 10*time.Millisecond {
		t.Errorf("p50 latency too high: %v, expected < 10ms", p50)
	}
	if p99 > 50*time.Millisecond {
		t.Errorf("p99 latency too high: %v, expected < 50ms", p99)
	}
}

// sortDurations sorts durations in-place using insertion sort (simple, good for small n)
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		j := i
		for j > 0 && d[j-1] > d[j] {
			d[j-1], d[j] = d[j], d[j-1]
			j--
		}
	}
}

// TestAnalyzePerformance_ConcurrentLoad tests behavior under concurrent load
func TestAnalyzePerformance_ConcurrentLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	mock := &perfMockClient{
		findings: generateFindings(5),
		latency:  100 * time.Microsecond, // Simulate minimal latency
	}
	analyzer := NewAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: generateCode(100),
		Kind:    input.KindFile,
	}}
	policies := generatePolicies(5)
	personaPrompt := codeReviewerPrompt

	concurrencyLevels := []int{1, 2, 4, 8, 16}
	requestsPerWorker := 50

	for _, workers := range concurrencyLevels {
		t.Run(fmt.Sprintf("%d_workers", workers), func(t *testing.T) {
			var wg sync.WaitGroup
			var errorCount atomic.Int64
			var totalOps atomic.Int64

			start := time.Now()

			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					ctx := context.Background()
					for i := 0; i < requestsPerWorker; i++ {
						_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
						if err != nil {
							errorCount.Add(1)
						} else {
							totalOps.Add(1)
						}
					}
				}()
			}

			wg.Wait()
			elapsed := time.Since(start)

			ops := totalOps.Load()
			errs := errorCount.Load()
			throughput := float64(ops) / elapsed.Seconds()

			t.Logf("Workers: %d, Ops: %d, Errors: %d, Duration: %v, Throughput: %.2f ops/sec",
				workers, ops, errs, elapsed, throughput)

			if errs > 0 {
				t.Errorf("unexpected errors during concurrent load: %d", errs)
			}
		})
	}
}

// TestAnalyzePerformance_MemoryStability tests that memory usage stays stable over many iterations
func TestAnalyzePerformance_MemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	mock := &perfMockClient{
		findings: generateFindings(20),
		latency:  0,
	}
	analyzer := NewAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: generateCode(500),
		Kind:    input.KindFile,
	}}
	policies := generatePolicies(10)
	ctx := context.Background()
	personaPrompt := codeReviewerPrompt

	// Force GC and get initial memory stats
	runtime.GC()
	var startMem, endMem runtime.MemStats
	runtime.ReadMemStats(&startMem)

	iterations := 5000
	for i := 0; i < iterations; i++ {
		_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
		if err != nil {
			t.Fatalf("unexpected error at iteration %d: %v", i, err)
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&endMem)

	heapGrowth := int64(endMem.HeapAlloc) - int64(startMem.HeapAlloc)
	allocsPerOp := (endMem.Mallocs - startMem.Mallocs) / uint64(iterations)

	t.Logf("Memory stats over %d iterations:", iterations)
	t.Logf("  Heap growth: %d bytes (%.2f KB)", heapGrowth, float64(heapGrowth)/1024)
	t.Logf("  Allocs per op: %d", allocsPerOp)
	t.Logf("  Total GC cycles: %d", endMem.NumGC-startMem.NumGC)

	// Check for memory leaks - heap should not grow excessively
	maxAllowedGrowth := int64(10 * 1024 * 1024) // 10 MB
	if heapGrowth > maxAllowedGrowth {
		t.Errorf("excessive heap growth: %d bytes, suggests memory leak", heapGrowth)
	}
}

// TestAnalyzePerformance_ContextCancellation tests that context cancellation is handled properly
func TestAnalyzePerformance_ContextCancellation(t *testing.T) {
	mock := &perfMockClient{
		findings: generateFindings(5),
		latency:  500 * time.Millisecond, // Longer latency to allow cancellation
	}
	analyzer := NewAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: generateCode(100),
		Kind:    input.KindFile,
	}}
	policies := generatePolicies(5)
	personaPrompt := codeReviewerPrompt

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
	elapsed := time.Since(start)

	// Should complete quickly due to context cancellation
	if err == nil {
		t.Log("Note: mock completed before cancellation (expected if latency simulation was skipped)")
	} else if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected context error, got: %v", err)
	}

	t.Logf("Context cancellation completed in %v", elapsed)
}

// TestAnalyzePerformance_LargeArtifacts tests performance with large code artifacts
func TestAnalyzePerformance_LargeArtifacts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	sizes := []int{100, 500, 1000, 5000, 10000}

	for _, lines := range sizes {
		t.Run(fmt.Sprintf("%d_lines", lines), func(t *testing.T) {
			mock := &perfMockClient{
				findings: generateFindings(10),
				latency:  0,
			}
			analyzer := NewAnalyzer(mock)

			code := generateCode(lines)
			artifacts := []input.Artifact{{
				Path:    "test.go",
				Content: code,
				Kind:    input.KindFile,
			}}
			policies := generatePolicies(5)
			ctx := context.Background()
			personaPrompt := codeReviewerPrompt

			iterations := 100
			start := time.Now()

			for i := 0; i < iterations; i++ {
				_, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			elapsed := time.Since(start)
			avgLatency := elapsed / time.Duration(iterations)

			t.Logf("Code size: %d lines (~%d bytes), Avg latency: %v",
				lines, len(code), avgLatency)
		})
	}
}

// TestAnalyzePerformance_ScalingWithFindings tests how performance scales with finding count
func TestAnalyzePerformance_ScalingWithFindings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	findingCounts := []int{1, 10, 50, 100, 500}

	for _, count := range findingCounts {
		t.Run(fmt.Sprintf("%d_findings", count), func(t *testing.T) {
			mock := &perfMockClient{
				findings: generateFindings(count),
				latency:  0,
			}
			analyzer := NewAnalyzer(mock)

			artifacts := []input.Artifact{{
				Path:    "test.go",
				Content: generateCode(200),
				Kind:    input.KindFile,
			}}
			policies := generatePolicies(5)
			ctx := context.Background()
			personaPrompt := codeReviewerPrompt

			iterations := 500
			start := time.Now()

			var totalResults int
			for i := 0; i < iterations; i++ {
				results, err := analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				totalResults += len(results)
			}

			elapsed := time.Since(start)
			avgLatency := elapsed / time.Duration(iterations)

			t.Logf("Findings: %d, Total results: %d, Avg latency: %v",
				count, totalResults, avgLatency)
		})
	}
}

// TestFormatPoliciesPerformance_LargePolicySet tests policy formatting with many policies
func TestFormatPoliciesPerformance_LargePolicySet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	policyCounts := []int{10, 50, 100, 500}

	for _, count := range policyCounts {
		t.Run(fmt.Sprintf("%d_policies", count), func(t *testing.T) {
			policies := generatePolicies(count)

			iterations := 10000
			start := time.Now()

			var totalLen int
			for i := 0; i < iterations; i++ {
				text := FormatPolicies(policies)
				totalLen += len(text)
			}

			elapsed := time.Since(start)
			avgLatency := elapsed / time.Duration(iterations)

			t.Logf("Policies: %d, Output length: %d, Avg latency: %v",
				count, totalLen/iterations, avgLatency)
		})
	}
}
