package metrics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewTiming(t *testing.T) {
	timing := NewTiming()
	if timing.queuedAt.IsZero() {
		t.Error("queuedAt should be set")
	}
	if !timing.startedAt.IsZero() {
		t.Error("startedAt should not be set initially")
	}
}

func TestTiming_QueueDuration(t *testing.T) {
	timing := NewTiming()
	time.Sleep(10 * time.Millisecond)
	timing.Start()
	
	qd := timing.QueueDuration()
	if qd < 10*time.Millisecond {
		t.Errorf("queue duration should be at least 10ms, got %v", qd)
	}
}

func TestTiming_AnalysisDuration(t *testing.T) {
	timing := NewTiming()
	timing.Start()
	time.Sleep(10 * time.Millisecond)
	timing.Complete()
	
	ad := timing.AnalysisDuration()
	if ad < 10*time.Millisecond {
		t.Errorf("analysis duration should be at least 10ms, got %v", ad)
	}
}

func TestCollector_Record(t *testing.T) {
	c := NewCollector()
	
	event := AnalysisEvent{
		ID:               "test-1",
		Timestamp:        time.Now(),
		Type:             AnalysisTypeFull,
		Tier:             TierComprehensive,
		FindingCount:     5,
		CacheResult:      CacheMiss,
		TokensIn:         100,
		TokensOut:        50,
	}
	
	c.Record(event)
	
	if c.counters.totalAnalyses.Load() != 1 {
		t.Error("total analyses should be 1")
	}
	if c.counters.totalFindings.Load() != 5 {
		t.Error("total findings should be 5")
	}
	if c.counters.cacheMisses.Load() != 1 {
		t.Error("cache misses should be 1")
	}
}

func TestCollector_RecordConcurrent(t *testing.T) {
	c := NewCollector()
	
	var wg sync.WaitGroup
	n := 100
	
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			event := AnalysisEvent{
				ID:           string(rune(id)),
				Timestamp:    time.Now(),
				Type:         AnalysisTypeFull,
				FindingCount: 1,
				CacheResult:  CacheHit,
			}
			c.Record(event)
		}(i)
	}
	
	wg.Wait()
	
	if c.counters.totalAnalyses.Load() != int64(n) {
		t.Errorf("expected %d analyses, got %d", n, c.counters.totalAnalyses.Load())
	}
}

func TestCollector_GetStats(t *testing.T) {
	c := NewCollector()
	
	// Record some events
	for i := 0; i < 10; i++ {
		event := AnalysisEvent{
			ID:               "test",
			Timestamp:        time.Now(),
			Type:             AnalysisTypeFull,
			Tier:             TierFast,
			AnalysisDuration: 100 * time.Millisecond,
			TotalDuration:    120 * time.Millisecond,
			FindingCount:     2,
			TokensIn:         500,
			TokensOut:        100,
			CacheResult:      CacheMiss,
		}
		c.Record(event)
	}
	
	// Add some cache hits
	for i := 0; i < 5; i++ {
		event := AnalysisEvent{
			ID:          "test-hit",
			Timestamp:   time.Now(),
			CacheResult: CacheHit,
		}
		c.Record(event)
	}
	
	stats := c.GetStats()
	
	if stats.TotalAnalyses != 15 {
		t.Errorf("expected 15 analyses, got %d", stats.TotalAnalyses)
	}
	if stats.CacheHits != 5 {
		t.Errorf("expected 5 cache hits, got %d", stats.CacheHits)
	}
	if stats.CacheMisses != 10 {
		t.Errorf("expected 10 cache misses, got %d", stats.CacheMisses)
	}
	
	// Check cache hit rate: 5 / 15 = 0.333...
	expectedHitRate := 5.0 / 15.0
	if stats.CacheHitRate < expectedHitRate-0.01 || stats.CacheHitRate > expectedHitRate+0.01 {
		t.Errorf("expected cache hit rate ~%.3f, got %.3f", expectedHitRate, stats.CacheHitRate)
	}
}

func TestCollector_GetRecentEvents(t *testing.T) {
	c := NewCollector()
	
	for i := 0; i < 20; i++ {
		c.Record(AnalysisEvent{
			ID:        string(rune('a' + i)),
			Timestamp: time.Now(),
		})
	}
	
	events := c.GetRecentEvents(5)
	if len(events) != 5 {
		t.Errorf("expected 5 events, got %d", len(events))
	}
	
	// Should return all if n > count
	events = c.GetRecentEvents(100)
	if len(events) != 20 {
		t.Errorf("expected 20 events, got %d", len(events))
	}
}

func TestCollector_Reset(t *testing.T) {
	c := NewCollector()
	
	c.Record(AnalysisEvent{ID: "test", FindingCount: 5})
	c.Reset()
	
	if c.counters.totalAnalyses.Load() != 0 {
		t.Error("counters should be reset")
	}
	
	events := c.GetRecentEvents(10)
	if len(events) != 0 {
		t.Error("events should be cleared")
	}
}

func TestCollector_Pruning(t *testing.T) {
	c := NewCollector(WithMaxEvents(100))
	
	// Add more than max events
	for i := 0; i < 150; i++ {
		c.Record(AnalysisEvent{
			ID:        string(rune(i)),
			Timestamp: time.Now(),
		})
	}
	
	c.mu.RLock()
	eventCount := len(c.events)
	c.mu.RUnlock()
	
	// After pruning, should be less than max + 10%
	if eventCount > 100 {
		t.Errorf("events should be pruned, got %d", eventCount)
	}
}

func TestRecorder_StartAnalysis(t *testing.T) {
	c := NewCollector()
	r := NewRecorder(c, "ollama", "qwen2.5-coder:7b")
	
	builder := r.StartAnalysis(AnalysisTypeFull, TierComprehensive)
	
	if builder.event.Provider != "ollama" {
		t.Error("provider should be set")
	}
	if builder.event.Model != "qwen2.5-coder:7b" {
		t.Error("model should be set")
	}
	if builder.event.ID == "" {
		t.Error("ID should be generated")
	}
}

func TestAnalysisBuilder_Complete(t *testing.T) {
	c := NewCollector()
	r := NewRecorder(c, "test", "test-model")
	
	builder := r.StartAnalysis(AnalysisTypeFull, TierFast)
	builder.WithFile("test.go", "package main\n\nfunc main() {}\n")
	builder.WithPolicies(3)
	builder.MarkStarted()
	
	time.Sleep(10 * time.Millisecond)
	builder.Complete(2, 100, 50)
	
	events := c.GetRecentEvents(1)
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
	
	e := events[0]
	if e.FindingCount != 2 {
		t.Errorf("expected 2 findings, got %d", e.FindingCount)
	}
	if e.TokensIn != 100 {
		t.Errorf("expected 100 tokens in, got %d", e.TokensIn)
	}
	if e.LineCount != 4 {
		t.Errorf("expected 4 lines, got %d", e.LineCount)
	}
	if e.PolicyCount != 3 {
		t.Errorf("expected 3 policies, got %d", e.PolicyCount)
	}
	if e.AnalysisDuration < 10*time.Millisecond {
		t.Error("analysis duration should be at least 10ms")
	}
}

func TestAnalysisBuilder_CompleteWithError(t *testing.T) {
	c := NewCollector()
	r := NewRecorder(c, "test", "test-model")
	
	builder := r.StartAnalysis(AnalysisTypeFull, TierComprehensive)
	builder.MarkStarted()
	builder.CompleteWithError(os.ErrNotExist)
	
	if c.counters.totalErrors.Load() != 1 {
		t.Error("error count should be 1")
	}
	
	events := c.GetRecentEvents(1)
	if events[0].Error == "" {
		t.Error("error should be recorded")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	key1 := GenerateCacheKey("code1", "policies", "persona")
	key2 := GenerateCacheKey("code1", "policies", "persona")
	key3 := GenerateCacheKey("code2", "policies", "persona")
	
	if key1 != key2 {
		t.Error("same inputs should produce same key")
	}
	if key1 == key3 {
		t.Error("different inputs should produce different keys")
	}
	if len(key1) != 32 {
		t.Errorf("expected 32 char hex key, got %d", len(key1))
	}
}

func TestExporter_WriteReport(t *testing.T) {
	c := NewCollector()
	
	// Add some data
	for i := 0; i < 5; i++ {
		c.Record(AnalysisEvent{
			Timestamp:        time.Now(),
			Type:             AnalysisTypeFull,
			Tier:             TierComprehensive,
			AnalysisDuration: 100 * time.Millisecond,
			FindingCount:     2,
			TokensIn:         500,
			TokensOut:        100,
			CacheResult:      CacheMiss,
		})
	}
	
	e := NewExporter(c)
	var buf bytes.Buffer
	err := e.WriteReport(&buf)
	if err != nil {
		t.Fatal(err)
	}
	
	report := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Total Analyses:")) {
		t.Error("report should contain summary")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Latency")) {
		t.Error("report should contain latency section")
	}
	t.Log(report)
}

func TestExporter_ExportJSON(t *testing.T) {
	c := NewCollector()
	c.Record(AnalysisEvent{
		ID:           "test",
		Timestamp:    time.Now(),
		Type:         AnalysisTypeFull,
		FindingCount: 3,
	})
	
	e := NewExporter(c)
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")
	
	err := e.ExportJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	
	var report struct {
		Stats  AggregateStats  `json:"stats"`
		Events []AnalysisEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	
	if len(report.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(report.Events))
	}
}

func TestExporter_WriteCSV(t *testing.T) {
	c := NewCollector()
	c.Record(AnalysisEvent{
		ID:           "test-1",
		Timestamp:    time.Now(),
		Type:         AnalysisTypeFull,
		Tier:         TierComprehensive,
		FilePath:     "test.go",
		FindingCount: 3,
		Provider:     "ollama",
		Model:        "qwen:7b",
	})
	
	e := NewExporter(c)
	var buf bytes.Buffer
	err := e.WriteCSV(&buf)
	if err != nil {
		t.Fatal(err)
	}
	
	csv := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("id,timestamp,type")) {
		t.Error("CSV should have header")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test-1")) {
		t.Error("CSV should contain event ID")
	}
	if !bytes.Contains(buf.Bytes(), []byte("ollama")) {
		t.Error("CSV should contain provider")
	}
	t.Log(csv)
}

func TestTierLevel_String(t *testing.T) {
	tests := []struct {
		tier TierLevel
		want string
	}{
		{TierInstant, "instant"},
		{TierFast, "fast"},
		{TierComprehensive, "comprehensive"},
		{TierLevel(99), "unknown"},
	}
	
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("TierLevel(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestNoOpRecorder(t *testing.T) {
	r := NoOpRecorder()
	
	// Should not panic
	builder := r.StartAnalysis(AnalysisTypeFull, TierComprehensive)
	builder.Complete(5, 100, 50)
	
	// Collector should have 0 max events, so nothing stored
	if r.collector.maxEvents != 0 {
		t.Error("NoOp collector should have 0 max events")
	}
}
