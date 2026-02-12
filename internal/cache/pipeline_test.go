package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPipeline_Basic(t *testing.T) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		return "result-" + item.ID, nil
	}
	
	p := NewPipeline(workFunc)
	p.Start()
	defer p.Stop()
	
	item := WorkItem{
		ID:       "test-1",
		Content:  "content",
		Policies: "policies",
		Persona:  "persona",
	}
	
	result := p.SubmitAndWait(context.Background(), item)
	
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Result != "result-test-1" {
		t.Errorf("expected result-test-1, got %v", result.Result)
	}
	if result.FromCache {
		t.Error("first result should not be from cache")
	}
}

func TestPipeline_CacheHit(t *testing.T) {
	var callCount atomic.Int32
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		callCount.Add(1)
		return "result", nil
	}
	
	p := NewPipeline(workFunc)
	p.Start()
	defer p.Stop()
	
	item := WorkItem{
		ID:       "test-1",
		Content:  "same-content",
		Policies: "same-policies",
		Persona:  "same-persona",
	}
	
	// First call - cache miss
	result1 := p.SubmitAndWait(context.Background(), item)
	if result1.FromCache {
		t.Error("first result should not be from cache")
	}
	
	// Second call with same content - should hit cache
	item.ID = "test-2" // Different ID, same content
	result2 := p.SubmitAndWait(context.Background(), item)
	if !result2.FromCache {
		t.Error("second result should be from cache")
	}
	
	// Work function should only be called once
	if callCount.Load() != 1 {
		t.Errorf("expected work func to be called once, got %d", callCount.Load())
	}
}

func TestPipeline_Error(t *testing.T) {
	expectedErr := errors.New("test error")
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		return nil, expectedErr
	}
	
	p := NewPipeline(workFunc)
	p.Start()
	defer p.Stop()
	
	item := WorkItem{ID: "test-1", Content: "content"}
	result := p.SubmitAndWait(context.Background(), item)
	
	if result.Error == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(result.Error, expectedErr) {
		t.Errorf("expected test error, got %v", result.Error)
	}
	
	stats := p.Stats()
	if stats.Errors != 1 {
		t.Errorf("expected 1 error in stats, got %d", stats.Errors)
	}
}

func TestPipeline_Concurrent(t *testing.T) {
	var callCount atomic.Int32
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		callCount.Add(1)
		time.Sleep(10 * time.Millisecond) // Simulate work
		return "result-" + item.ID, nil
	}
	
	p := NewPipeline(workFunc, WithWorkers(4))
	p.Start()
	defer p.Stop()
	
	var wg sync.WaitGroup
	n := 20
	results := make([]WorkResult, n)
	
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			item := WorkItem{
				ID:       string(rune('a' + i)),
				Content:  string(rune('a' + i%5)), // 5 unique contents
				Policies: "policies",
				Persona:  "persona",
			}
			results[i] = p.SubmitAndWait(context.Background(), item)
		}(i)
	}
	
	wg.Wait()
	
	// Check all results
	for i, r := range results {
		if r.Error != nil {
			t.Errorf("result %d had error: %v", i, r.Error)
		}
	}
	
	// Should have cache hits (5 unique contents, 20 requests)
	stats := p.Stats()
	if stats.CacheHits == 0 {
		t.Error("expected some cache hits")
	}
	t.Logf("Stats: processed=%d, cacheHits=%d, errors=%d", 
		stats.Processed, stats.CacheHits, stats.Errors)
}

func TestPipeline_ContextCancellation(t *testing.T) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		select {
		case <-time.After(5 * time.Second):
			return "result", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	
	p := NewPipeline(workFunc)
	p.Start()
	defer p.Stop()
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	item := WorkItem{ID: "test-1", Content: "content"}
	result := p.SubmitAndWait(ctx, item)
	
	if result.Error == nil {
		t.Error("expected timeout error")
	}
}

func TestPipeline_Stats(t *testing.T) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		return "result", nil
	}
	
	c := New(WithMaxSize(100))
	p := NewPipeline(workFunc, WithCache(c))
	p.Start()
	defer p.Stop()
	
	// Process some items
	for i := 0; i < 5; i++ {
		item := WorkItem{
			ID:      string(rune('a' + i)),
			Content: string(rune('a' + i%2)), // 2 unique contents
		}
		p.SubmitAndWait(context.Background(), item)
	}
	
	stats := p.Stats()
	
	if stats.Processed < 2 {
		t.Errorf("expected at least 2 processed, got %d", stats.Processed)
	}
	if stats.CacheHits < 3 {
		t.Errorf("expected at least 3 cache hits, got %d", stats.CacheHits)
	}
	if stats.CacheStats.Size != 2 {
		t.Errorf("expected 2 cache entries, got %d", stats.CacheStats.Size)
	}
}

func TestPipeline_Stop(t *testing.T) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		time.Sleep(100 * time.Millisecond)
		return "result", nil
	}
	
	p := NewPipeline(workFunc)
	p.Start()
	
	// Submit an item
	item := WorkItem{ID: "test-1", Content: "content"}
	resultChan := p.Submit(item)
	
	// Stop immediately
	p.Stop()
	
	// Should either complete or timeout gracefully
	select {
	case <-resultChan:
		// OK - completed
	case <-time.After(200 * time.Millisecond):
		// OK - stopped before completion
	}
}

func TestPipeline_DoubleStart(t *testing.T) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		return "result", nil
	}
	
	p := NewPipeline(workFunc)
	
	// Should not panic
	p.Start()
	p.Start()
	p.Stop()
	p.Stop()
}

func TestPipeline_QueueDepth(t *testing.T) {
	blockChan := make(chan struct{})
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		<-blockChan // Block until released
		return "result", nil
	}
	
	p := NewPipeline(workFunc, WithWorkers(1), WithQueueSize(10))
	p.Start()
	defer p.Stop()
	
	// Submit multiple items without waiting
	for i := 0; i < 5; i++ {
		item := WorkItem{
			ID:      string(rune('a' + i)),
			Content: string(rune('a' + i)), // All unique
		}
		p.Submit(item)
	}
	
	// Give time for items to queue
	time.Sleep(10 * time.Millisecond)
	
	stats := p.Stats()
	// Queue depth should be > 0 since worker is blocked
	t.Logf("Queue depth: %d", stats.QueueDepth)
	
	// Release worker
	close(blockChan)
}

func BenchmarkPipeline_Submit(b *testing.B) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		return "result", nil
	}
	
	p := NewPipeline(workFunc, WithWorkers(4))
	p.Start()
	defer p.Stop()
	
	b.ReportAllocs()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		item := WorkItem{
			ID:      string(rune(i % 1000)),
			Content: string(rune(i % 100)), // 100 unique contents
		}
		<-p.Submit(item) // Wait for result
	}
}

func BenchmarkPipeline_CacheHit(b *testing.B) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		return "result", nil
	}
	
	p := NewPipeline(workFunc)
	p.Start()
	defer p.Stop()
	
	// Pre-warm cache
	item := WorkItem{
		ID:      "warmup",
		Content: "same-content",
	}
	p.SubmitAndWait(context.Background(), item)
	
	b.ReportAllocs()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		item := WorkItem{
			ID:      string(rune(i)),
			Content: "same-content", // Always cache hit
		}
		<-p.Submit(item)
	}
}

func BenchmarkPipeline_Parallel(b *testing.B) {
	workFunc := func(ctx context.Context, item WorkItem) (interface{}, error) {
		return "result", nil
	}
	
	p := NewPipeline(workFunc, WithWorkers(8))
	p.Start()
	defer p.Stop()
	
	var counter atomic.Int64
	
	b.ReportAllocs()
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := counter.Add(1)
			item := WorkItem{
				ID:      fmt.Sprintf("item-%d", id),
				Content: string(rune(id % 50)), // 50 unique contents
			}
			<-p.Submit(item)
		}
	})
}
