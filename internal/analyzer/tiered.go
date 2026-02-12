package analyzer

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/metrics"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// Tier represents an analysis tier level
type Tier int

const (
	// TierInstant provides immediate feedback from cache and pattern matching (~0-100ms)
	TierInstant Tier = iota
	// TierFast uses a fast local model for quick analysis (~100ms-2s)
	TierFast
	// TierComprehensive performs full LLM analysis (~2-30s)
	TierComprehensive
)

func (t Tier) String() string {
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

// TieredResult represents a result from a specific tier
type TieredResult struct {
	Tier      Tier
	FilePath  string
	Results   []sarif.Result
	Error     error
	FromCache bool
	Duration  time.Duration
}

// TieredAnalyzer provides progressive analysis across multiple tiers
type TieredAnalyzer struct {
	// Tier analyzers
	cache            *cache.Cache
	instantPatterns  []PatternRule
	fastClient       BAMLClient // Optional fast/local model
	comprehensiveClient BAMLClient // Full model

	// Configuration
	fastModel       string
	fastEnabled     bool
	instantEnabled  bool

	// Metrics
	metricsCollector *metrics.Collector
	metricsEnabled   bool

	// Stats
	instantHits       atomic.Int64
	instantMisses     atomic.Int64
	fastCalls         atomic.Int64
	comprehensiveCalls atomic.Int64

	mu sync.RWMutex
}

// PatternRule defines a regex-based instant check
type PatternRule struct {
	ID          string
	Pattern     *regexp.Regexp
	Level       string
	Message     string
	Explanation string
	Confidence  float64
}

// TieredAnalyzerOption configures a TieredAnalyzer
type TieredAnalyzerOption func(*TieredAnalyzer)

// WithInstantPatterns sets custom instant-check patterns
func WithInstantPatterns(patterns []PatternRule) TieredAnalyzerOption {
	return func(ta *TieredAnalyzer) {
		ta.instantPatterns = patterns
	}
}

// WithFastClient sets the fast-tier client (e.g., local Ollama)
func WithFastClient(client BAMLClient) TieredAnalyzerOption {
	return func(ta *TieredAnalyzer) {
		ta.fastClient = client
		ta.fastEnabled = client != nil
	}
}

// WithTieredCache sets a custom cache
func WithTieredCache(c *cache.Cache) TieredAnalyzerOption {
	return func(ta *TieredAnalyzer) {
		ta.cache = c
	}
}

// WithInstantEnabled enables/disables instant tier
func WithInstantEnabled(enabled bool) TieredAnalyzerOption {
	return func(ta *TieredAnalyzer) {
		ta.instantEnabled = enabled
	}
}

// WithMetricsCollector enables metrics collection
func WithMetricsCollector(collector *metrics.Collector) TieredAnalyzerOption {
	return func(ta *TieredAnalyzer) {
		ta.metricsCollector = collector
		ta.metricsEnabled = collector != nil
	}
}

// NewTieredAnalyzer creates a new tiered analyzer
func NewTieredAnalyzer(comprehensiveClient BAMLClient, opts ...TieredAnalyzerOption) *TieredAnalyzer {
	ta := &TieredAnalyzer{
		cache:               cache.New(cache.WithMaxSize(1000), cache.WithTTL(1*time.Hour)),
		instantPatterns:     defaultPatterns(),
		comprehensiveClient: comprehensiveClient,
		instantEnabled:      true,
		fastEnabled:         false,
	}

	for _, opt := range opts {
		opt(ta)
	}

	return ta
}

// defaultPatterns returns built-in instant-check patterns
func defaultPatterns() []PatternRule {
	return []PatternRule{
		// Go-specific patterns
		{
			ID:          "error-ignored",
			Pattern:     regexp.MustCompile(`(?m)^\s*[a-zA-Z_][a-zA-Z0-9_]*\s*,\s*_\s*:?=`),
			Level:       "warning",
			Message:     "Error return value is being ignored",
			Explanation: "Ignoring errors can lead to silent failures and unexpected behavior",
			Confidence:  0.7,
		},
		{
			ID:          "empty-error-check",
			Pattern:     regexp.MustCompile(`if\s+err\s*!=\s*nil\s*\{\s*\}`),
			Level:       "warning",
			Message:     "Empty error handling block",
			Explanation: "Error is checked but not handled, which may hide failures",
			Confidence:  0.9,
		},
		{
			ID:          "todo-fixme",
			Pattern:     regexp.MustCompile(`(?i)(TODO|FIXME|HACK|XXX):`),
			Level:       "note",
			Message:     "Code contains TODO/FIXME comment",
			Explanation: "There is unfinished work or a known issue marked in the code",
			Confidence:  1.0,
		},
		{
			ID:          "hardcoded-credentials",
			Pattern:     regexp.MustCompile(`(?i)(password|secret|api_key|apikey|token)\s*[:=]\s*["'][^"']+["']`),
			Level:       "error",
			Message:     "Possible hardcoded credentials detected",
			Explanation: "Hardcoded secrets are a security risk and should be moved to environment variables or a secrets manager",
			Confidence:  0.8,
		},
		{
			ID:          "sql-string-concat",
			Pattern:     regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE).*\+.*["']`),
			Level:       "error",
			Message:     "Possible SQL injection vulnerability",
			Explanation: "String concatenation in SQL queries can lead to SQL injection attacks",
			Confidence:  0.7,
		},
		{
			ID:          "fmt-errorf-wrap",
			Pattern:     regexp.MustCompile(`fmt\.Errorf\([^%]*%s[^%]*,\s*err\)`),
			Level:       "note",
			Message:     "Consider using %w to wrap errors",
			Explanation: "Using %w instead of %s preserves the error chain for errors.Is/As",
			Confidence:  0.6,
		},
		// Generic patterns
		{
			ID:          "debug-print",
			Pattern:     regexp.MustCompile(`(?m)^\s*(fmt\.Print|console\.log|print\(|System\.out\.print)`),
			Level:       "note",
			Message:     "Debug print statement found",
			Explanation: "Debug print statements should be removed before committing",
			Confidence:  0.8,
		},
		{
			ID:          "commented-code",
			Pattern:     regexp.MustCompile(`(?m)^\s*//\s*(if|for|func|return|var|const)\s+`),
			Level:       "note",
			Message:     "Commented-out code detected",
			Explanation: "Commented code should be removed; version control preserves history",
			Confidence:  0.6,
		},
	}
}

// AnalyzeProgressive returns a channel that emits results progressively from each tier
func (ta *TieredAnalyzer) AnalyzeProgressive(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) <-chan TieredResult {
	resultChan := make(chan TieredResult, len(artifacts)*3) // Up to 3 tiers per artifact

	go func() {
		defer close(resultChan)

		policyText := FormatPolicies(policies)

		for _, art := range artifacts {
			select {
			case <-ctx.Done():
				resultChan <- TieredResult{
					Tier:     TierInstant,
					FilePath: art.Path,
					Error:    ctx.Err(),
				}
				return
			default:
			}

			// Tier 1: Instant (cache + patterns)
			if ta.instantEnabled {
				ta.runInstantTier(ctx, art, policyText, personaPrompt, resultChan)
			}

			// Tier 2: Fast (if enabled)
			if ta.fastEnabled && ta.fastClient != nil {
				ta.runFastTier(ctx, art, policies, personaPrompt, resultChan)
			}

			// Tier 3: Comprehensive
			ta.runComprehensiveTier(ctx, art, policies, personaPrompt, policyText, resultChan)
		}
	}()

	return resultChan
}

// runInstantTier executes instant-tier analysis
func (ta *TieredAnalyzer) runInstantTier(ctx context.Context, art input.Artifact, policyText, personaPrompt string, resultChan chan<- TieredResult) {
	start := time.Now()
	cacheKey := cache.ContentKey(art.Content, policyText, personaPrompt)

	// Check cache first
	if cached, ok := ta.cache.Get(cacheKey); ok {
		ta.instantHits.Add(1)
		duration := time.Since(start)
		
		ta.recordMetrics(art, metrics.TierInstant, duration, 0, metrics.CacheHit, nil)
		
		if results, ok := cached.([]sarif.Result); ok {
			resultChan <- TieredResult{
				Tier:      TierInstant,
				FilePath:  art.Path,
				Results:   results,
				FromCache: true,
				Duration:  duration,
			}
			return
		}
	}

	ta.instantMisses.Add(1)

	// Run pattern matching
	results := ta.runPatternMatching(art)
	duration := time.Since(start)
	
	ta.recordMetrics(art, metrics.TierInstant, duration, len(results), metrics.CacheMiss, nil)

	resultChan <- TieredResult{
		Tier:      TierInstant,
		FilePath:  art.Path,
		Results:   results,
		FromCache: false,
		Duration:  duration,
	}
}

// runPatternMatching executes regex-based instant checks
func (ta *TieredAnalyzer) runPatternMatching(art input.Artifact) []sarif.Result {
	var results []sarif.Result
	lines := strings.Split(art.Content, "\n")

	for _, pattern := range ta.instantPatterns {
		matches := pattern.Pattern.FindAllStringIndex(art.Content, -1)
		for _, match := range matches {
			// Calculate line number from byte offset
			lineNum := 1
			for i := range lines {
				if match[0] <= len(strings.Join(lines[:i+1], "\n")) {
					lineNum = i + 1
					break
				}
			}

			results = append(results, sarif.Result{
				RuleID:  pattern.ID,
				Level:   pattern.Level,
				Message: sarif.Message{Text: pattern.Message},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: art.Path},
						Region:           sarif.Region{StartLine: lineNum, EndLine: lineNum},
					},
				}},
				Properties: map[string]interface{}{
					"gavel/explanation": pattern.Explanation,
					"gavel/confidence":  pattern.Confidence,
					"gavel/tier":        "instant",
				},
			})
		}
	}

	return results
}

// runFastTier executes fast-tier analysis with local model
func (ta *TieredAnalyzer) runFastTier(ctx context.Context, art input.Artifact, policies map[string]config.Policy, personaPrompt string, resultChan chan<- TieredResult) {
	start := time.Now()
	ta.fastCalls.Add(1)

	analyzer := NewAnalyzer(ta.fastClient)
	results, err := analyzer.Analyze(ctx, []input.Artifact{art}, policies, personaPrompt)
	duration := time.Since(start)

	// Tag results with tier
	for i := range results {
		if results[i].Properties == nil {
			results[i].Properties = make(map[string]interface{})
		}
		results[i].Properties["gavel/tier"] = "fast"
	}

	ta.recordMetrics(art, metrics.TierFast, duration, len(results), metrics.CacheMiss, err)

	resultChan <- TieredResult{
		Tier:     TierFast,
		FilePath: art.Path,
		Results:  results,
		Error:    err,
		Duration: duration,
	}
}

// runComprehensiveTier executes full LLM analysis
func (ta *TieredAnalyzer) runComprehensiveTier(ctx context.Context, art input.Artifact, policies map[string]config.Policy, personaPrompt, policyText string, resultChan chan<- TieredResult) {
	start := time.Now()
	cacheKey := cache.ContentKey(art.Content, policyText, personaPrompt)

	ta.comprehensiveCalls.Add(1)

	analyzer := NewAnalyzer(ta.comprehensiveClient)
	results, err := analyzer.Analyze(ctx, []input.Artifact{art}, policies, personaPrompt)
	duration := time.Since(start)

	if err == nil {
		// Cache successful results
		ta.cache.Set(cacheKey, results)
	}

	// Tag results with tier
	for i := range results {
		if results[i].Properties == nil {
			results[i].Properties = make(map[string]interface{})
		}
		results[i].Properties["gavel/tier"] = "comprehensive"
	}

	ta.recordMetrics(art, metrics.TierComprehensive, duration, len(results), metrics.CacheMiss, err)

	resultChan <- TieredResult{
		Tier:     TierComprehensive,
		FilePath: art.Path,
		Results:  results,
		Error:    err,
		Duration: duration,
	}
}

// recordMetrics records an analysis event to the metrics collector
func (ta *TieredAnalyzer) recordMetrics(art input.Artifact, tier metrics.TierLevel, duration time.Duration, findingCount int, cacheResult metrics.CacheResult, err error) {
	if !ta.metricsEnabled || ta.metricsCollector == nil {
		return
	}

	event := metrics.AnalysisEvent{
		Timestamp:        time.Now(),
		Type:             metrics.AnalysisTypeFull,
		Tier:             tier,
		FilePath:         art.Path,
		FileSize:         len(art.Content),
		LineCount:        countLines(art.Content),
		AnalysisDuration: duration,
		TotalDuration:    duration,
		FindingCount:     findingCount,
		CacheResult:      cacheResult,
	}

	if err != nil {
		event.Error = err.Error()
		event.ErrorCount = 1
	}

	ta.metricsCollector.Record(event)
}

// countLines counts the number of lines in content
func countLines(content string) int {
	count := 1
	for _, c := range content {
		if c == '\n' {
			count++
		}
	}
	return count
}

// Analyze performs a single-shot analysis using all enabled tiers
// Returns combined results from all tiers (deduplicated)
func (ta *TieredAnalyzer) Analyze(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) ([]sarif.Result, error) {
	var allResults []sarif.Result
	var lastError error

	for result := range ta.AnalyzeProgressive(ctx, artifacts, policies, personaPrompt) {
		if result.Error != nil {
			lastError = result.Error
			continue
		}
		allResults = append(allResults, result.Results...)
	}

	// Deduplicate results (prefer higher-tier results)
	deduplicated := ta.deduplicateResults(allResults)

	return deduplicated, lastError
}

// deduplicateResults removes duplicate findings, preferring higher-tier results
func (ta *TieredAnalyzer) deduplicateResults(results []sarif.Result) []sarif.Result {
	// Key: ruleID + file + line
	seen := make(map[string]sarif.Result)
	tierPriority := map[string]int{"comprehensive": 3, "fast": 2, "instant": 1}

	for _, r := range results {
		if len(r.Locations) == 0 {
			continue
		}

		loc := r.Locations[0].PhysicalLocation
		key := r.RuleID + "|" + loc.ArtifactLocation.URI + "|" + string(rune(loc.Region.StartLine))

		tier := "instant"
		if t, ok := r.Properties["gavel/tier"].(string); ok {
			tier = t
		}

		if existing, ok := seen[key]; ok {
			existingTier := "instant"
			if t, ok := existing.Properties["gavel/tier"].(string); ok {
				existingTier = t
			}
			// Keep higher-tier result
			if tierPriority[tier] > tierPriority[existingTier] {
				seen[key] = r
			}
		} else {
			seen[key] = r
		}
	}

	deduplicated := make([]sarif.Result, 0, len(seen))
	for _, r := range seen {
		deduplicated = append(deduplicated, r)
	}

	return deduplicated
}

// TieredAnalyzerStats holds statistics for the tiered analyzer
type TieredAnalyzerStats struct {
	InstantHits        int64            `json:"instant_hits"`
	InstantMisses      int64            `json:"instant_misses"`
	FastCalls          int64            `json:"fast_calls"`
	ComprehensiveCalls int64            `json:"comprehensive_calls"`
	CacheStats         cache.CacheStats `json:"cache_stats"`
}

// Stats returns current statistics
func (ta *TieredAnalyzer) Stats() TieredAnalyzerStats {
	return TieredAnalyzerStats{
		InstantHits:        ta.instantHits.Load(),
		InstantMisses:      ta.instantMisses.Load(),
		FastCalls:          ta.fastCalls.Load(),
		ComprehensiveCalls: ta.comprehensiveCalls.Load(),
		CacheStats:         ta.cache.Stats(),
	}
}

// ClearCache clears the analysis cache
func (ta *TieredAnalyzer) ClearCache() {
	ta.cache.Clear()
}

// AddPattern adds a custom pattern rule for instant checks
func (ta *TieredAnalyzer) AddPattern(rule PatternRule) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	ta.instantPatterns = append(ta.instantPatterns, rule)
}

// SetPatterns replaces all pattern rules
func (ta *TieredAnalyzer) SetPatterns(rules []PatternRule) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	ta.instantPatterns = rules
}
