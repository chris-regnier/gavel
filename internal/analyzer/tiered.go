package analyzer

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/chris-regnier/gavel/internal/astcheck"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/metrics"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
)

var analyzerTracer = otel.Tracer("github.com/chris-regnier/gavel/internal/analyzer")

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
	instantPatterns  []rules.Rule
	fastClient       BAMLClient // Optional fast/local model
	comprehensiveClient BAMLClient // Full model

	// AST analysis
	astRegistry *astcheck.Registry

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

// TieredAnalyzerOption configures a TieredAnalyzer
type TieredAnalyzerOption func(*TieredAnalyzer)

// WithInstantPatterns sets custom instant-check patterns
func WithInstantPatterns(patterns []rules.Rule) TieredAnalyzerOption {
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
		astRegistry:         astcheck.DefaultRegistry(),
		instantEnabled:      true,
		fastEnabled:         false,
	}

	for _, opt := range opts {
		opt(ta)
	}

	return ta
}

// defaultPatterns returns built-in instant-check patterns based on industry standards (CWE, OWASP, SonarQube)
func defaultPatterns() []rules.Rule {
	r, err := rules.DefaultRules()
	if err != nil {
		panic("loading embedded default rules: " + err.Error())
	}
	return r
}

// AnalyzeProgressive returns a channel that emits results progressively from each tier.
// Instant-tier results for ALL artifacts are emitted first (providing immediate feedback),
// followed by fast and comprehensive tiers per artifact.
func (ta *TieredAnalyzer) AnalyzeProgressive(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) <-chan TieredResult {
	resultChan := make(chan TieredResult, len(artifacts)*3) // Up to 3 tiers per artifact

	go func() {
		defer close(resultChan)

		policyText := FormatPolicies(policies)

		// Phase 1: Run instant tier for ALL artifacts first (~0-100ms total)
		if ta.instantEnabled {
			instantCtx, instantSpan := analyzerTracer.Start(ctx, "run instant tier",
				trace.WithAttributes(
					attribute.String("gavel.tier", "instant"),
					attribute.Int("gavel.rule_count", len(ta.instantPatterns)),
				),
			)
			for _, art := range artifacts {
				select {
				case <-instantCtx.Done():
					resultChan <- TieredResult{
						Tier:     TierInstant,
						FilePath: art.Path,
						Error:    instantCtx.Err(),
					}
					instantSpan.End()
					return
				default:
				}
				ta.runInstantTier(instantCtx, art, policyText, personaPrompt, resultChan)
			}
			instantSpan.End()
		}

		// Phase 2a: Run fast tier if enabled
		if ta.fastEnabled && ta.fastClient != nil {
			fastCtx, fastSpan := analyzerTracer.Start(ctx, "run fast tier",
				trace.WithAttributes(
					attribute.String("gavel.tier", "fast"),
				),
			)
			for _, art := range artifacts {
				select {
				case <-fastCtx.Done():
					resultChan <- TieredResult{
						Tier:     TierFast,
						FilePath: art.Path,
						Error:    fastCtx.Err(),
					}
					fastSpan.End()
					return
				default:
				}
				ta.runFastTier(fastCtx, art, policies, personaPrompt, resultChan)
			}
			fastSpan.End()
		}

		// Phase 2b: Run comprehensive tier
		comprehensiveCtx, comprehensiveSpan := analyzerTracer.Start(ctx, "run comprehensive tier",
			trace.WithAttributes(
				attribute.String("gavel.tier", "comprehensive"),
			),
		)
		for _, art := range artifacts {
			select {
			case <-comprehensiveCtx.Done():
				resultChan <- TieredResult{
					Tier:     TierComprehensive,
					FilePath: art.Path,
					Error:    comprehensiveCtx.Err(),
				}
				comprehensiveSpan.End()
				return
			default:
			}
			ta.runComprehensiveTier(comprehensiveCtx, art, policies, personaPrompt, policyText, resultChan)
		}
		comprehensiveSpan.End()
	}()

	return resultChan
}

// runInstantTier executes instant-tier analysis
func (ta *TieredAnalyzer) runInstantTier(ctx context.Context, art input.Artifact, policyText, personaPrompt string, resultChan chan<- TieredResult) {
	ctx, span := analyzerTracer.Start(ctx, "analyze file",
		trace.WithAttributes(
			attribute.String("gavel.file_path", art.Path),
			attribute.Int("gavel.file_size", len(art.Content)),
			attribute.Int("gavel.line_count", countLines(art.Content)),
			attribute.String("gavel.tier", "instant"),
		),
	)
	defer span.End()

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

	span.SetAttributes(attribute.Int("gavel.finding_count", len(results)))

	resultChan <- TieredResult{
		Tier:      TierInstant,
		FilePath:  art.Path,
		Results:   results,
		FromCache: false,
		Duration:  duration,
	}
}

// runPatternMatching executes instant checks by partitioning rules into regex and AST types
func (ta *TieredAnalyzer) runPatternMatching(art input.Artifact) []sarif.Result {
	ta.mu.RLock()
	patterns := ta.instantPatterns
	ta.mu.RUnlock()

	var regexRules, astRules []rules.Rule
	for _, rule := range patterns {
		switch rule.Type {
		case rules.RuleTypeAST:
			astRules = append(astRules, rule)
		default:
			regexRules = append(regexRules, rule)
		}
	}

	results := ta.runRegexRules(art, regexRules)
	results = append(results, ta.runASTRules(art, astRules)...)
	return results
}

// runRegexRules executes regex-based instant checks using industry-standard rules
func (ta *TieredAnalyzer) runRegexRules(art input.Artifact, regexRules []rules.Rule) []sarif.Result {
	var results []sarif.Result
	lines := strings.Split(art.Content, "\n")

	for _, rule := range regexRules {
		// Skip rules that don't apply to this file's language
		if len(rule.Languages) > 0 && !matchesLanguage(art.Path, rule.Languages) {
			continue
		}

		matches := rule.Pattern.FindAllStringIndex(art.Content, -1)
		for _, match := range matches {
			// Calculate line number from byte offset
			lineNum := 1
			for i := range lines {
				if match[0] <= len(strings.Join(lines[:i+1], "\n")) {
					lineNum = i + 1
					break
				}
			}

			props := map[string]interface{}{
				"gavel/explanation":  rule.Explanation,
				"gavel/confidence":   rule.Confidence,
				"gavel/tier":         "instant",
				"gavel/rule-source":  string(rule.Source),
			}

			// Add standard references if available
			if len(rule.CWE) > 0 {
				props["gavel/cwe"] = rule.CWE
			}
			if len(rule.OWASP) > 0 {
				props["gavel/owasp"] = rule.OWASP
			}
			if rule.Remediation != "" {
				props["gavel/remediation"] = rule.Remediation
			}
			if len(rule.References) > 0 {
				props["gavel/references"] = rule.References
			}

			results = append(results, sarif.Result{
				RuleID:  rule.ID,
				Level:   rule.Level,
				Message: sarif.Message{Text: rule.Message},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: art.Path},
						Region:           sarif.Region{StartLine: lineNum, EndLine: lineNum},
					},
				}},
				Properties: props,
			})
		}
	}

	return results
}

// runASTRules executes tree-sitter AST-based instant checks
func (ta *TieredAnalyzer) runASTRules(art input.Artifact, astRules []rules.Rule) []sarif.Result {
	if len(astRules) == 0 {
		return nil
	}

	lang, langName, ok := astcheck.Detect(art.Path)
	if !ok {
		return nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(art.Content))
	if err != nil {
		return nil
	}

	var results []sarif.Result
	sourceBytes := []byte(art.Content)

	for _, rule := range astRules {
		if len(rule.Languages) > 0 && !matchesLanguage(art.Path, rule.Languages) {
			continue
		}

		check, ok := ta.astRegistry.Get(rule.ASTCheck)
		if !ok {
			continue
		}

		matches := check.Run(tree, sourceBytes, langName, rule.ASTConfig)
		for _, m := range matches {
			msg := rule.Message
			if m.Message != "" {
				msg = m.Message
			}

			props := map[string]interface{}{
				"gavel/explanation": rule.Explanation,
				"gavel/confidence":  rule.Confidence,
				"gavel/tier":        "instant",
				"gavel/rule-type":   "ast",
				"gavel/rule-source": string(rule.Source),
			}
			if len(rule.CWE) > 0 {
				props["gavel/cwe"] = rule.CWE
			}
			if len(rule.OWASP) > 0 {
				props["gavel/owasp"] = rule.OWASP
			}
			if rule.Remediation != "" {
				props["gavel/remediation"] = rule.Remediation
			}
			if len(rule.References) > 0 {
				props["gavel/references"] = rule.References
			}
			if m.Extra != nil {
				for k, v := range m.Extra {
					props["gavel/"+k] = v
				}
			}

			results = append(results, sarif.Result{
				RuleID:  rule.ID,
				Level:   rule.Level,
				Message: sarif.Message{Text: msg},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: art.Path},
						Region:           sarif.Region{StartLine: m.StartLine, EndLine: m.EndLine},
					},
				}},
				Properties: props,
			})
		}
	}
	return results
}

// matchesLanguage checks if a file path matches any of the specified languages
func matchesLanguage(path string, languages []string) bool {
	for _, lang := range languages {
		switch lang {
		case "go":
			if strings.HasSuffix(path, ".go") {
				return true
			}
		case "java":
			if strings.HasSuffix(path, ".java") {
				return true
			}
		case "python":
			if strings.HasSuffix(path, ".py") {
				return true
			}
		case "javascript", "js":
			if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".jsx") {
				return true
			}
		case "typescript", "ts":
			if strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx") {
				return true
			}
		case "c":
			if strings.HasSuffix(path, ".c") || strings.HasSuffix(path, ".h") {
				return true
			}
		case "rust":
			if strings.HasSuffix(path, ".rs") {
				return true
			}
		}
	}
	return false
}

// runFastTier executes fast-tier analysis with local model
func (ta *TieredAnalyzer) runFastTier(ctx context.Context, art input.Artifact, policies map[string]config.Policy, personaPrompt string, resultChan chan<- TieredResult) {
	ctx, span := analyzerTracer.Start(ctx, "analyze file",
		trace.WithAttributes(
			attribute.String("gavel.file_path", art.Path),
			attribute.Int("gavel.file_size", len(art.Content)),
			attribute.Int("gavel.line_count", countLines(art.Content)),
			attribute.String("gavel.tier", "fast"),
		),
	)
	defer span.End()

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

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.SetAttributes(attribute.Int("gavel.finding_count", len(results)))

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
	ctx, span := analyzerTracer.Start(ctx, "analyze file",
		trace.WithAttributes(
			attribute.String("gavel.file_path", art.Path),
			attribute.Int("gavel.file_size", len(art.Content)),
			attribute.Int("gavel.line_count", countLines(art.Content)),
			attribute.String("gavel.tier", "comprehensive"),
		),
	)
	defer span.End()

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

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.SetAttributes(attribute.Int("gavel.finding_count", len(results)))

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
		key := r.RuleID + "|" + loc.ArtifactLocation.URI + "|" + strconv.Itoa(loc.Region.StartLine)

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
func (ta *TieredAnalyzer) AddPattern(rule rules.Rule) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	ta.instantPatterns = append(ta.instantPatterns, rule)
}

// SetPatterns replaces all pattern rules
func (ta *TieredAnalyzer) SetPatterns(r []rules.Rule) {
	ta.mu.Lock()
	defer ta.mu.Unlock()
	ta.instantPatterns = r
}
