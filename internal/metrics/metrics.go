package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// AnalysisType identifies the type of analysis performed
type AnalysisType string

const (
	AnalysisTypeFull      AnalysisType = "full"
	AnalysisTypeChunk     AnalysisType = "chunk"
	AnalysisTypeIncremental AnalysisType = "incremental"
)

// CacheResult indicates whether a cache lookup was a hit or miss
type CacheResult string

const (
	CacheHit  CacheResult = "hit"
	CacheMiss CacheResult = "miss"
	CacheStale CacheResult = "stale"
)

// TierLevel identifies the analysis tier
type TierLevel int

const (
	TierInstant TierLevel = iota
	TierFast
	TierComprehensive
)

func (t TierLevel) String() string {
	switch t {
	case TierInstant:
		return "instant"
	case TierFast:
		return "fast"
	case TierComprehensive:
		return "comprehensive"
	default:
		return "unknown"
	}
}

// AnalysisEvent captures metrics for a single analysis operation
type AnalysisEvent struct {
	// Identification
	ID        string       `json:"id"`
	Timestamp time.Time    `json:"timestamp"`
	Type      AnalysisType `json:"type"`
	Tier      TierLevel    `json:"tier"`

	// Input characteristics
	FilePath     string `json:"file_path"`
	FileSize     int    `json:"file_size"`      // bytes
	LineCount    int    `json:"line_count"`
	ChunkCount   int    `json:"chunk_count"`    // if chunked
	PolicyCount  int    `json:"policy_count"`

	// Timing
	QueueDuration    time.Duration `json:"queue_duration"`    // time in queue
	AnalysisDuration time.Duration `json:"analysis_duration"` // LLM call time
	TotalDuration    time.Duration `json:"total_duration"`    // end-to-end

	// Results
	FindingCount int     `json:"finding_count"`
	ErrorCount   int     `json:"error_count"`
	TokensIn     int     `json:"tokens_in"`
	TokensOut    int     `json:"tokens_out"`

	// Cache
	CacheResult CacheResult `json:"cache_result"`
	CacheKey    string      `json:"cache_key,omitempty"`

	// Provider info
	Provider string `json:"provider"`
	Model    string `json:"model"`

	// Error tracking
	Error string `json:"error,omitempty"`
}

// AnalysisTiming is a helper for tracking analysis timing
type AnalysisTiming struct {
	queuedAt   time.Time
	startedAt  time.Time
	completedAt time.Time
}

// NewTiming creates a new timing tracker, marking queue time as now
func NewTiming() *AnalysisTiming {
	return &AnalysisTiming{
		queuedAt: time.Now(),
	}
}

// Start marks the analysis as started (dequeued)
func (t *AnalysisTiming) Start() {
	t.startedAt = time.Now()
}

// Complete marks the analysis as completed
func (t *AnalysisTiming) Complete() {
	t.completedAt = time.Now()
}

// QueueDuration returns time spent in queue
func (t *AnalysisTiming) QueueDuration() time.Duration {
	if t.startedAt.IsZero() {
		return 0
	}
	return t.startedAt.Sub(t.queuedAt)
}

// AnalysisDuration returns time spent in analysis
func (t *AnalysisTiming) AnalysisDuration() time.Duration {
	if t.completedAt.IsZero() || t.startedAt.IsZero() {
		return 0
	}
	return t.completedAt.Sub(t.startedAt)
}

// TotalDuration returns total end-to-end time
func (t *AnalysisTiming) TotalDuration() time.Duration {
	if t.completedAt.IsZero() {
		return 0
	}
	return t.completedAt.Sub(t.queuedAt)
}

// AggregateStats holds computed aggregate statistics
type AggregateStats struct {
	// Counts
	TotalAnalyses int64 `json:"total_analyses"`
	TotalErrors   int64 `json:"total_errors"`
	TotalFindings int64 `json:"total_findings"`

	// Latency stats (in milliseconds for JSON readability)
	AvgAnalysisDurationMs float64 `json:"avg_analysis_duration_ms"`
	P50AnalysisDurationMs float64 `json:"p50_analysis_duration_ms"`
	P95AnalysisDurationMs float64 `json:"p95_analysis_duration_ms"`
	P99AnalysisDurationMs float64 `json:"p99_analysis_duration_ms"`
	MaxAnalysisDurationMs float64 `json:"max_analysis_duration_ms"`

	AvgQueueDurationMs float64 `json:"avg_queue_duration_ms"`
	AvgTotalDurationMs float64 `json:"avg_total_duration_ms"`

	// Cache stats
	CacheHits     int64   `json:"cache_hits"`
	CacheMisses   int64   `json:"cache_misses"`
	CacheStale    int64   `json:"cache_stale"`
	CacheHitRate  float64 `json:"cache_hit_rate"`

	// Throughput
	AnalysesPerMinute float64 `json:"analyses_per_minute"`
	FindingsPerAnalysis float64 `json:"findings_per_analysis"`

	// Token usage
	TotalTokensIn  int64   `json:"total_tokens_in"`
	TotalTokensOut int64   `json:"total_tokens_out"`
	AvgTokensIn    float64 `json:"avg_tokens_in"`
	AvgTokensOut   float64 `json:"avg_tokens_out"`

	// By tier breakdown
	ByTier map[string]*TierStats `json:"by_tier"`

	// Time window
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

// TierStats holds stats for a specific tier
type TierStats struct {
	Count                 int64   `json:"count"`
	AvgAnalysisDurationMs float64 `json:"avg_analysis_duration_ms"`
	ErrorRate             float64 `json:"error_rate"`
}

// atomicCounters holds atomic counters for real-time stats
type atomicCounters struct {
	totalAnalyses atomic.Int64
	totalErrors   atomic.Int64
	totalFindings atomic.Int64
	cacheHits     atomic.Int64
	cacheMisses   atomic.Int64
	cacheStale    atomic.Int64
	tokensIn      atomic.Int64
	tokensOut     atomic.Int64
}

// Collector collects and stores analysis metrics
type Collector struct {
	mu       sync.RWMutex
	events   []AnalysisEvent
	counters atomicCounters

	// Configuration
	maxEvents   int
	windowSize  time.Duration
	
	// Start time for throughput calculation
	startTime time.Time
}

// CollectorOption configures a Collector
type CollectorOption func(*Collector)

// WithMaxEvents sets the maximum number of events to retain
func WithMaxEvents(n int) CollectorOption {
	return func(c *Collector) {
		c.maxEvents = n
	}
}

// WithWindowSize sets the time window for aggregate stats
func WithWindowSize(d time.Duration) CollectorOption {
	return func(c *Collector) {
		c.windowSize = d
	}
}

// NewCollector creates a new metrics collector
func NewCollector(opts ...CollectorOption) *Collector {
	c := &Collector{
		events:     make([]AnalysisEvent, 0, 1000),
		maxEvents:  10000,
		windowSize: 1 * time.Hour,
		startTime:  time.Now(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Record adds an analysis event to the collector
func (c *Collector) Record(event AnalysisEvent) {
	// Update atomic counters
	c.counters.totalAnalyses.Add(1)
	c.counters.totalFindings.Add(int64(event.FindingCount))
	c.counters.tokensIn.Add(int64(event.TokensIn))
	c.counters.tokensOut.Add(int64(event.TokensOut))

	if event.Error != "" {
		c.counters.totalErrors.Add(1)
	}

	switch event.CacheResult {
	case CacheHit:
		c.counters.cacheHits.Add(1)
	case CacheMiss:
		c.counters.cacheMisses.Add(1)
	case CacheStale:
		c.counters.cacheStale.Add(1)
	}

	// Store event
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = append(c.events, event)

	// Prune old events if needed
	if len(c.events) > c.maxEvents {
		// Remove oldest 10%
		pruneCount := c.maxEvents / 10
		c.events = c.events[pruneCount:]
	}
}

// GetStats computes aggregate statistics from collected events
func (c *Collector) GetStats() AggregateStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-c.windowSize)

	stats := AggregateStats{
		TotalAnalyses: c.counters.totalAnalyses.Load(),
		TotalErrors:   c.counters.totalErrors.Load(),
		TotalFindings: c.counters.totalFindings.Load(),
		CacheHits:     c.counters.cacheHits.Load(),
		CacheMisses:   c.counters.cacheMisses.Load(),
		CacheStale:    c.counters.cacheStale.Load(),
		TotalTokensIn:  c.counters.tokensIn.Load(),
		TotalTokensOut: c.counters.tokensOut.Load(),
		ByTier:        make(map[string]*TierStats),
		WindowStart:   windowStart,
		WindowEnd:     now,
	}

	// Calculate cache hit rate
	totalCacheOps := stats.CacheHits + stats.CacheMisses + stats.CacheStale
	if totalCacheOps > 0 {
		stats.CacheHitRate = float64(stats.CacheHits) / float64(totalCacheOps)
	}

	// Calculate averages
	if stats.TotalAnalyses > 0 {
		stats.AvgTokensIn = float64(stats.TotalTokensIn) / float64(stats.TotalAnalyses)
		stats.AvgTokensOut = float64(stats.TotalTokensOut) / float64(stats.TotalAnalyses)
		stats.FindingsPerAnalysis = float64(stats.TotalFindings) / float64(stats.TotalAnalyses)
	}

	// Filter events within window and compute detailed stats
	var windowEvents []AnalysisEvent
	for _, e := range c.events {
		if e.Timestamp.After(windowStart) {
			windowEvents = append(windowEvents, e)
		}
	}

	if len(windowEvents) == 0 {
		return stats
	}

	// Compute latency stats from window events
	durations := make([]float64, 0, len(windowEvents))
	var sumAnalysis, sumQueue, sumTotal float64
	tierCounts := make(map[TierLevel]int64)
	tierDurations := make(map[TierLevel]float64)
	tierErrors := make(map[TierLevel]int64)

	for _, e := range windowEvents {
		ms := float64(e.AnalysisDuration.Milliseconds())
		durations = append(durations, ms)
		sumAnalysis += ms
		sumQueue += float64(e.QueueDuration.Milliseconds())
		sumTotal += float64(e.TotalDuration.Milliseconds())

		tierCounts[e.Tier]++
		tierDurations[e.Tier] += ms
		if e.Error != "" {
			tierErrors[e.Tier]++
		}
	}

	n := float64(len(windowEvents))
	stats.AvgAnalysisDurationMs = sumAnalysis / n
	stats.AvgQueueDurationMs = sumQueue / n
	stats.AvgTotalDurationMs = sumTotal / n

	// Sort for percentiles
	sortFloat64s(durations)
	stats.P50AnalysisDurationMs = percentile(durations, 0.50)
	stats.P95AnalysisDurationMs = percentile(durations, 0.95)
	stats.P99AnalysisDurationMs = percentile(durations, 0.99)
	stats.MaxAnalysisDurationMs = durations[len(durations)-1]

	// Throughput
	elapsed := now.Sub(c.startTime).Minutes()
	if elapsed > 0 {
		stats.AnalysesPerMinute = float64(stats.TotalAnalyses) / elapsed
	}

	// Per-tier stats
	for tier, count := range tierCounts {
		tierStats := &TierStats{
			Count: count,
		}
		if count > 0 {
			tierStats.AvgAnalysisDurationMs = tierDurations[tier] / float64(count)
			tierStats.ErrorRate = float64(tierErrors[tier]) / float64(count)
		}
		stats.ByTier[tier.String()] = tierStats
	}

	return stats
}

// GetRecentEvents returns the most recent n events
func (c *Collector) GetRecentEvents(n int) []AnalysisEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if n > len(c.events) {
		n = len(c.events)
	}
	if n <= 0 {
		return nil
	}

	// Return copy of most recent events
	result := make([]AnalysisEvent, n)
	copy(result, c.events[len(c.events)-n:])
	return result
}

// Reset clears all collected metrics
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = c.events[:0]
	c.counters = atomicCounters{}
	c.startTime = time.Now()
}

// sortFloat64s sorts a slice of float64 in place
func sortFloat64s(s []float64) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1] > s[j] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}

// percentile returns the value at the given percentile (0.0-1.0)
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}
