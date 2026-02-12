package analyzer

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
)

type countingMockClient struct {
	findings  []Finding
	callCount atomic.Int32
}

func (m *countingMockClient) AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error) {
	m.callCount.Add(1)
	return m.findings, nil
}

func TestCachedAnalyzer_Basic(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{
			RuleID:     "test-rule",
			Level:      "warning",
			Message:    "Test finding",
			FilePath:   "test.go",
			StartLine:  1,
			EndLine:    1,
			Confidence: 0.9,
		}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	results, err := ca.Analyze(context.Background(), artifacts, policies, "test persona")
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if mock.callCount.Load() != 1 {
		t.Errorf("expected 1 call, got %d", mock.callCount.Load())
	}
}

func TestCachedAnalyzer_CacheHit(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{
			RuleID:  "test-rule",
			Level:   "warning",
			Message: "Test finding",
		}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}
	persona := "test persona"

	// First call - cache miss
	_, err := ca.Analyze(context.Background(), artifacts, policies, persona)
	if err != nil {
		t.Fatal(err)
	}

	// Second call - should hit cache
	_, err = ca.Analyze(context.Background(), artifacts, policies, persona)
	if err != nil {
		t.Fatal(err)
	}

	// Mock should only be called once
	if mock.callCount.Load() != 1 {
		t.Errorf("expected 1 call (cache hit), got %d", mock.callCount.Load())
	}

	stats := ca.Stats()
	if stats.CacheHits != 1 {
		t.Errorf("expected 1 cache hit, got %d", stats.CacheHits)
	}
	if stats.CacheMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", stats.CacheMisses)
	}
}

func TestCachedAnalyzer_DifferentContent(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}
	persona := "test persona"

	// First content
	artifacts1 := []input.Artifact{{Path: "test.go", Content: "package main"}}
	_, _ = ca.Analyze(context.Background(), artifacts1, policies, persona)

	// Different content - should not hit cache
	artifacts2 := []input.Artifact{{Path: "test.go", Content: "package other"}}
	_, _ = ca.Analyze(context.Background(), artifacts2, policies, persona)

	if mock.callCount.Load() != 2 {
		t.Errorf("expected 2 calls (different content), got %d", mock.callCount.Load())
	}
}

func TestCachedAnalyzer_MultipleArtifacts(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{
		{Path: "file1.go", Content: "package one"},
		{Path: "file2.go", Content: "package two"},
		{Path: "file3.go", Content: "package three"},
	}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	results, err := ca.Analyze(context.Background(), artifacts, policies, "persona")
	if err != nil {
		t.Fatal(err)
	}

	// Should have findings from all 3 files
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Should call mock 3 times (once per artifact)
	if mock.callCount.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", mock.callCount.Load())
	}

	// Second call - all should hit cache
	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")

	if mock.callCount.Load() != 3 {
		t.Errorf("expected still 3 calls (all cache hits), got %d", mock.callCount.Load())
	}
}

func TestCachedAnalyzer_AsyncAnalysis(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{
		{Path: "file1.go", Content: "package one"},
		{Path: "file2.go", Content: "package two"},
	}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	resultChan := ca.AnalyzeAsync(context.Background(), artifacts, policies, "persona")

	var results []AsyncResult
	for r := range resultChan {
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Error != nil {
			t.Errorf("unexpected error for %s: %v", r.FilePath, r.Error)
		}
	}
}

func TestCachedAnalyzer_AsyncCacheHit(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{
		{Path: "file1.go", Content: "package one"},
	}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	// First call to populate cache
	for range ca.AnalyzeAsync(context.Background(), artifacts, policies, "persona") {
	}

	// Second call should hit cache
	var cacheHitCount int
	for r := range ca.AnalyzeAsync(context.Background(), artifacts, policies, "persona") {
		if r.FromCache {
			cacheHitCount++
		}
	}

	if cacheHitCount != 1 {
		t.Errorf("expected 1 cache hit, got %d", cacheHitCount)
	}
}

func TestCachedAnalyzer_Stats(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{{Path: "test.go", Content: "package main"}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	// 1 miss
	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")
	// 2 hits
	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")
	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")

	stats := ca.Stats()

	if stats.TotalCalls != 3 {
		t.Errorf("expected 3 total calls, got %d", stats.TotalCalls)
	}
	if stats.CacheHits != 2 {
		t.Errorf("expected 2 cache hits, got %d", stats.CacheHits)
	}
	if stats.CacheMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", stats.CacheMisses)
	}

	expectedHitRate := 2.0 / 3.0
	if stats.HitRate < expectedHitRate-0.01 || stats.HitRate > expectedHitRate+0.01 {
		t.Errorf("expected hit rate ~%.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}

func TestCachedAnalyzer_ClearCache(t *testing.T) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{{Path: "test.go", Content: "package main"}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	// Populate cache
	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")

	// Clear cache
	ca.ClearCache()

	// Should miss again
	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")

	if mock.callCount.Load() != 2 {
		t.Errorf("expected 2 calls after clear, got %d", mock.callCount.Load())
	}
}

func TestCachedAnalyzer_CustomCache(t *testing.T) {
	mock := &countingMockClient{findings: []Finding{}}

	customCache := cache.New(cache.WithMaxSize(10), cache.WithTTL(1*time.Second))
	ca := NewCachedAnalyzer(mock, WithAnalyzerCache(customCache))
	defer ca.Close()

	artifacts := []input.Artifact{{Path: "test.go", Content: "package main"}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")

	stats := ca.Stats()
	if stats.CacheStats.MaxSize != 10 {
		t.Errorf("expected max size 10, got %d", stats.CacheStats.MaxSize)
	}
}

func BenchmarkCachedAnalyzer_CacheHit(b *testing.B) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	artifacts := []input.Artifact{{Path: "test.go", Content: "package main"}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	// Populate cache
	_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")
	}
}

func BenchmarkCachedAnalyzer_CacheMiss(b *testing.B) {
	mock := &countingMockClient{
		findings: []Finding{{RuleID: "test-rule"}},
	}

	ca := NewCachedAnalyzer(mock)
	defer ca.Close()

	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Different content each time = cache miss
		artifacts := []input.Artifact{{
			Path:    "test.go",
			Content: string(rune(i % 1000)),
		}}
		_, _ = ca.Analyze(context.Background(), artifacts, policies, "persona")
	}
}
