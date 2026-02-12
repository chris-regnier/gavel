package metrics

import (
	"sync"
	"testing"
	"time"
)

func BenchmarkCollector_Record(b *testing.B) {
	c := NewCollector()
	event := AnalysisEvent{
		ID:               "bench-test",
		Timestamp:        time.Now(),
		Type:             AnalysisTypeFull,
		Tier:             TierComprehensive,
		FilePath:         "test.go",
		FileSize:         1000,
		LineCount:        50,
		AnalysisDuration: 100 * time.Millisecond,
		TotalDuration:    120 * time.Millisecond,
		FindingCount:     5,
		TokensIn:         500,
		TokensOut:        100,
		CacheResult:      CacheMiss,
		Provider:         "ollama",
		Model:            "qwen2.5-coder:7b",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		c.Record(event)
	}
}

func BenchmarkCollector_RecordParallel(b *testing.B) {
	c := NewCollector()
	event := AnalysisEvent{
		ID:           "bench-test",
		Timestamp:    time.Now(),
		Type:         AnalysisTypeFull,
		FindingCount: 5,
		CacheResult:  CacheMiss,
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Record(event)
		}
	})
}

func BenchmarkCollector_GetStats(b *testing.B) {
	c := NewCollector()
	
	// Pre-populate with events
	for i := 0; i < 1000; i++ {
		c.Record(AnalysisEvent{
			Timestamp:        time.Now(),
			Type:             AnalysisTypeFull,
			Tier:             TierLevel(i % 3),
			AnalysisDuration: time.Duration(i) * time.Millisecond,
			FindingCount:     i % 10,
			CacheResult:      CacheResult([]CacheResult{CacheHit, CacheMiss, CacheStale}[i%3]),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = c.GetStats()
	}
}

func BenchmarkCollector_GetRecentEvents(b *testing.B) {
	c := NewCollector()
	
	for i := 0; i < 1000; i++ {
		c.Record(AnalysisEvent{
			ID:        string(rune(i)),
			Timestamp: time.Now(),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = c.GetRecentEvents(100)
	}
}

func BenchmarkRecorder_FullWorkflow(b *testing.B) {
	c := NewCollector()
	r := NewRecorder(c, "ollama", "qwen2.5-coder:7b")
	
	code := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := r.StartAnalysis(AnalysisTypeFull, TierComprehensive)
		builder.WithFile("test.go", code)
		builder.WithPolicies(5)
		builder.WithCacheResult(CacheMiss, "abc123")
		builder.MarkStarted()
		builder.Complete(3, 500, 100)
	}
}

func BenchmarkRecorder_FullWorkflowParallel(b *testing.B) {
	c := NewCollector()
	r := NewRecorder(c, "ollama", "qwen2.5-coder:7b")
	
	code := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			builder := r.StartAnalysis(AnalysisTypeFull, TierComprehensive)
			builder.WithFile("test.go", code)
			builder.WithPolicies(5)
			builder.MarkStarted()
			builder.Complete(3, 500, 100)
		}
	})
}

func BenchmarkGenerateCacheKey(b *testing.B) {
	content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	policies := "- error-handling [warning]: Check error handling\n"
	persona := "You are a code reviewer..."

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = GenerateCacheKey(content, policies, persona)
	}
}

func BenchmarkTiming(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		t := NewTiming()
		t.Start()
		t.Complete()
		_ = t.QueueDuration()
		_ = t.AnalysisDuration()
		_ = t.TotalDuration()
	}
}

func BenchmarkExporter_WriteReport(b *testing.B) {
	c := NewCollector()
	
	// Populate with data
	for i := 0; i < 500; i++ {
		c.Record(AnalysisEvent{
			Timestamp:        time.Now(),
			Type:             AnalysisTypeFull,
			Tier:             TierLevel(i % 3),
			AnalysisDuration: time.Duration(i) * time.Millisecond,
			FindingCount:     i % 10,
			TokensIn:         500,
			TokensOut:        100,
			CacheResult:      CacheResult([]CacheResult{CacheHit, CacheMiss}[i%2]),
		})
	}

	e := NewExporter(c)
	
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf nullWriter
		_ = e.WriteReport(&buf)
	}
}

// nullWriter discards all writes
type nullWriter struct{}

func (nullWriter) Write(p []byte) (int, error) { return len(p), nil }

// TestMetricsOverhead measures the overhead of metrics collection
// compared to a baseline without metrics
func TestMetricsOverhead(t *testing.T) {
	iterations := 10000

	// Baseline: no metrics
	start := time.Now()
	for i := 0; i < iterations; i++ {
		// Simulate minimal work
		_ = i * 2
	}
	baseline := time.Since(start)

	// With metrics
	c := NewCollector()
	r := NewRecorder(c, "test", "test")

	start = time.Now()
	for i := 0; i < iterations; i++ {
		builder := r.StartAnalysis(AnalysisTypeFull, TierComprehensive)
		builder.MarkStarted()
		builder.Complete(1, 100, 50)
		_ = i * 2
	}
	withMetrics := time.Since(start)

	overhead := withMetrics - baseline
	overheadPerOp := overhead / time.Duration(iterations)

	t.Logf("Baseline: %v (%v/op)", baseline, baseline/time.Duration(iterations))
	t.Logf("With metrics: %v (%v/op)", withMetrics, withMetrics/time.Duration(iterations))
	t.Logf("Overhead: %v (%v/op)", overhead, overheadPerOp)

	// Metrics overhead should be < 10 microseconds per operation
	if overheadPerOp > 10*time.Microsecond {
		t.Errorf("metrics overhead too high: %v/op, want < 10µs/op", overheadPerOp)
	}
}

// TestConcurrentMetricsStability tests that metrics remain consistent under concurrent load
func TestConcurrentMetricsStability(t *testing.T) {
	c := NewCollector()
	r := NewRecorder(c, "test", "test")

	workers := 10
	opsPerWorker := 1000

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				builder := r.StartAnalysis(AnalysisTypeFull, TierComprehensive)
				builder.WithFile("test.go", "code")
				builder.MarkStarted()
				builder.Complete(1, 100, 50)
			}
		}()
	}

	wg.Wait()

	expectedTotal := int64(workers * opsPerWorker)
	actualTotal := c.counters.totalAnalyses.Load()

	if actualTotal != expectedTotal {
		t.Errorf("expected %d analyses, got %d", expectedTotal, actualTotal)
	}

	stats := c.GetStats()
	if stats.TotalAnalyses != expectedTotal {
		t.Errorf("stats total analyses mismatch: expected %d, got %d", expectedTotal, stats.TotalAnalyses)
	}
}
