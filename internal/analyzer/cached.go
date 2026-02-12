package analyzer

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// CachedAnalyzer wraps an Analyzer with caching and async support
type CachedAnalyzer struct {
	analyzer *Analyzer
	cache    *cache.Cache
	pipeline *cache.Pipeline

	// Stats
	cacheHits   atomic.Int64
	cacheMisses atomic.Int64
	totalCalls  atomic.Int64
}

// CachedAnalyzerOption configures a CachedAnalyzer
type CachedAnalyzerOption func(*CachedAnalyzer)

// WithAnalyzerCache sets a custom cache
func WithAnalyzerCache(c *cache.Cache) CachedAnalyzerOption {
	return func(ca *CachedAnalyzer) {
		ca.cache = c
	}
}

// WithAsyncWorkers enables async processing with n workers
func WithAsyncWorkers(n int) CachedAnalyzerOption {
	return func(ca *CachedAnalyzer) {
		if ca.pipeline != nil {
			ca.pipeline.Stop()
		}
		ca.pipeline = cache.NewPipeline(
			ca.workFunc,
			cache.WithWorkers(n),
			cache.WithCache(ca.cache),
		)
		ca.pipeline.Start()
	}
}

// NewCachedAnalyzer creates a new cached analyzer
func NewCachedAnalyzer(client BAMLClient, opts ...CachedAnalyzerOption) *CachedAnalyzer {
	ca := &CachedAnalyzer{
		analyzer: NewAnalyzer(client),
		cache:    cache.New(cache.WithMaxSize(500), cache.WithTTL(30*time.Minute)),
	}

	for _, opt := range opts {
		opt(ca)
	}

	return ca
}

// workFunc is the pipeline work function for async processing
func (ca *CachedAnalyzer) workFunc(ctx context.Context, item cache.WorkItem) (interface{}, error) {
	// Parse the stored data
	artifacts := []input.Artifact{{
		Path:    item.FilePath,
		Content: item.Content,
		Kind:    input.KindFile,
	}}

	// Policies are stored as formatted text, we need to create a minimal policy map
	// In practice, the pipeline should store the full policy map
	// For now, we'll use a simplified approach
	policies := map[string]config.Policy{
		"analysis": {
			Instruction: item.Policies,
			Enabled:     true,
		},
	}

	return ca.analyzer.Analyze(ctx, artifacts, policies, item.Persona)
}

// Analyze performs cached analysis
func (ca *CachedAnalyzer) Analyze(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) ([]sarif.Result, error) {
	ca.totalCalls.Add(1)

	// For single artifact, use simple caching
	if len(artifacts) == 1 {
		return ca.analyzeSingle(ctx, artifacts[0], policies, personaPrompt)
	}

	// For multiple artifacts, analyze each and combine results
	var allResults []sarif.Result
	for _, art := range artifacts {
		results, err := ca.analyzeSingle(ctx, art, policies, personaPrompt)
		if err != nil {
			return nil, err
		}
		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// analyzeSingle analyzes a single artifact with caching
func (ca *CachedAnalyzer) analyzeSingle(ctx context.Context, artifact input.Artifact, policies map[string]config.Policy, personaPrompt string) ([]sarif.Result, error) {
	policyText := FormatPolicies(policies)
	cacheKey := cache.ContentKey(artifact.Content, policyText, personaPrompt)

	// Check cache
	if cached, ok := ca.cache.Get(cacheKey); ok {
		ca.cacheHits.Add(1)
		if results, ok := cached.([]sarif.Result); ok {
			return results, nil
		}
	}

	ca.cacheMisses.Add(1)

	// Use pipeline if available
	if ca.pipeline != nil {
		item := cache.WorkItem{
			ID:       cacheKey,
			FilePath: artifact.Path,
			Content:  artifact.Content,
			Policies: policyText,
			Persona:  personaPrompt,
			CacheKey: cacheKey,
		}

		result := ca.pipeline.SubmitAndWait(ctx, item)
		if result.Error != nil {
			return nil, result.Error
		}

		if results, ok := result.Result.([]sarif.Result); ok {
			return results, nil
		}
	}

	// Synchronous analysis
	results, err := ca.analyzer.Analyze(ctx, []input.Artifact{artifact}, policies, personaPrompt)
	if err != nil {
		return nil, err
	}

	// Cache the result
	ca.cache.Set(cacheKey, results)

	return results, nil
}

// AnalyzeAsync submits artifacts for async analysis and returns a channel for results
func (ca *CachedAnalyzer) AnalyzeAsync(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) <-chan AsyncResult {
	resultChan := make(chan AsyncResult, len(artifacts))

	go func() {
		defer close(resultChan)

		policyText := FormatPolicies(policies)

		for _, art := range artifacts {
			cacheKey := cache.ContentKey(art.Content, policyText, personaPrompt)

			// Check cache first
			if cached, ok := ca.cache.Get(cacheKey); ok {
				ca.cacheHits.Add(1)
				if results, ok := cached.([]sarif.Result); ok {
					resultChan <- AsyncResult{
						FilePath:  art.Path,
						Results:   results,
						FromCache: true,
					}
					continue
				}
			}

			ca.cacheMisses.Add(1)

			// Perform analysis
			results, err := ca.analyzer.Analyze(ctx, []input.Artifact{art}, policies, personaPrompt)
			if err != nil {
				resultChan <- AsyncResult{
					FilePath: art.Path,
					Error:    err,
				}
				continue
			}

			// Cache the result
			ca.cache.Set(cacheKey, results)

			resultChan <- AsyncResult{
				FilePath:  art.Path,
				Results:   results,
				FromCache: false,
			}
		}
	}()

	return resultChan
}

// AsyncResult represents the result of an async analysis
type AsyncResult struct {
	FilePath  string
	Results   []sarif.Result
	Error     error
	FromCache bool
	Duration  time.Duration
}

// CachedAnalyzerStats holds statistics for the cached analyzer
type CachedAnalyzerStats struct {
	TotalCalls  int64          `json:"total_calls"`
	CacheHits   int64          `json:"cache_hits"`
	CacheMisses int64          `json:"cache_misses"`
	HitRate     float64        `json:"hit_rate"`
	CacheStats  cache.CacheStats `json:"cache_stats"`
}

// Stats returns current statistics
func (ca *CachedAnalyzer) Stats() CachedAnalyzerStats {
	hits := ca.cacheHits.Load()
	misses := ca.cacheMisses.Load()
	total := hits + misses

	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CachedAnalyzerStats{
		TotalCalls:  ca.totalCalls.Load(),
		CacheHits:   hits,
		CacheMisses: misses,
		HitRate:     hitRate,
		CacheStats:  ca.cache.Stats(),
	}
}

// ClearCache clears the analysis cache
func (ca *CachedAnalyzer) ClearCache() {
	ca.cache.Clear()
}

// Close stops the pipeline and cleans up resources
func (ca *CachedAnalyzer) Close() {
	if ca.pipeline != nil {
		ca.pipeline.Stop()
	}
}
