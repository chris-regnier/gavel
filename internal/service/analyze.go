package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/suppression"
)

// ClientFactory creates a BAMLClient from provider config.
type ClientFactory func(cfg config.ProviderConfig) analyzer.BAMLClient

// AnalyzeService orchestrates code/prose analysis.
type AnalyzeService struct {
	store         store.Store
	clientFactory ClientFactory
}

// NewAnalyzeService creates an AnalyzeService with the default BAML client factory.
func NewAnalyzeService(s store.Store) *AnalyzeService {
	return &AnalyzeService{
		store: s,
		clientFactory: func(cfg config.ProviderConfig) analyzer.BAMLClient {
			return analyzer.NewBAMLLiveClient(cfg)
		},
	}
}

// WithClientFactory overrides the client factory (for testing).
func (s *AnalyzeService) WithClientFactory(f ClientFactory) *AnalyzeService {
	s.clientFactory = f
	return s
}

// Analyze runs all tiers synchronously and stores the SARIF result.
func (s *AnalyzeService) Analyze(ctx context.Context, req AnalyzeRequest) (*AnalyzeResult, error) {
	personaPrompt, err := buildPersonaPrompt(ctx, req.Config)
	if err != nil {
		return nil, err
	}

	ta := analyzer.NewTieredAnalyzer(s.clientFactory(req.Config.Provider), tieredOptions(req.Rules)...)
	results, err := ta.Analyze(ctx, req.Artifacts, req.Config.Policies, personaPrompt)
	if err != nil {
		return nil, fmt.Errorf("analyzing: %w", err)
	}

	sarifLog := sarif.Assemble(results, BuildDescriptors(req.Config.Policies, req.Rules), scopeFromArtifacts(req.Artifacts), req.Config.Persona)

	baselineSummary, err := s.applyBaseline(ctx, sarifLog, req.BaselineID)
	if err != nil {
		return nil, err
	}

	suppressedCount := applySuppressions(sarifLog, req.SuppressionDir)

	resultID, err := s.store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return nil, fmt.Errorf("storing SARIF: %w", err)
	}

	return &AnalyzeResult{
		ResultID:      resultID,
		TotalFindings: countFindings(sarifLog),
		Suppressed:    suppressedCount,
		Baseline:      baselineSummary,
	}, nil
}

// AnalyzeScoped runs a diff-style scoped analysis: the instant tier
// runs against the full file artifact while the comprehensive tier
// runs against a window around the changed lines. Findings outside
// the changed-line range are filtered out before assembly. This is the
// shared entrypoint used by MCP's analyze_diff tool.
func (s *AnalyzeService) AnalyzeScoped(ctx context.Context, req ScopedAnalyzeRequest) (*AnalyzeResult, error) {
	if req.ChangedStart <= 0 || req.ChangedEnd <= 0 || req.ChangedStart > req.ChangedEnd {
		return nil, fmt.Errorf("invalid changed range [%d, %d]", req.ChangedStart, req.ChangedEnd)
	}

	personaPrompt, err := buildPersonaPrompt(ctx, req.Config)
	if err != nil {
		return nil, err
	}

	ta := analyzer.NewTieredAnalyzer(s.clientFactory(req.Config.Provider), tieredOptions(req.Rules)...)

	// Instant tier on the full file, then filter to the changed range.
	fullArtifact := input.Artifact{Path: req.Artifact.Path, Content: req.Artifact.Content, Kind: input.KindFile}
	instantResults := filterByLineRange(ta.RunPatternMatching(fullArtifact), req.ChangedStart, req.ChangedEnd)

	// Comprehensive tier on a window around the changed range.
	contextWindow := req.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 10
	}
	scopedContent, scopeStart := windowedContent(req.Artifact.Content, req.ChangedStart, req.ChangedEnd, contextWindow)
	scopedArtifact := []input.Artifact{{Path: req.Artifact.Path, Content: scopedContent, Kind: input.KindFile}}
	comprehensiveResults, err := ta.Analyze(ctx, scopedArtifact, req.Config.Policies, personaPrompt)
	if err != nil {
		return nil, fmt.Errorf("analyzing: %w", err)
	}

	// The LLM saw the scope starting at line 1; shift back to the
	// real file line numbers, then drop anything outside the changed
	// range. The instant tier already used real line numbers.
	offset := scopeStart - 1
	for i := range comprehensiveResults {
		if len(comprehensiveResults[i].Locations) > 0 {
			comprehensiveResults[i].Locations[0].PhysicalLocation.Region.StartLine += offset
			comprehensiveResults[i].Locations[0].PhysicalLocation.Region.EndLine += offset
		}
	}
	comprehensiveResults = filterByLineRange(comprehensiveResults, req.ChangedStart, req.ChangedEnd)

	allResults := append(instantResults, comprehensiveResults...)
	sarifLog := sarif.Assemble(allResults, BuildDescriptors(req.Config.Policies, req.Rules), "diff", req.Config.Persona)

	baselineSummary, err := s.applyBaseline(ctx, sarifLog, req.BaselineID)
	if err != nil {
		return nil, err
	}

	suppressedCount := applySuppressions(sarifLog, req.SuppressionDir)

	resultID, err := s.store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return nil, fmt.Errorf("storing SARIF: %w", err)
	}

	return &AnalyzeResult{
		ResultID:      resultID,
		TotalFindings: countFindings(sarifLog),
		Suppressed:    suppressedCount,
		Baseline:      baselineSummary,
	}, nil
}

// applyBaseline stamps automation details onto sarifLog and, when
// baselineRef is non-empty, loads that baseline (by stored ID or file
// path) and annotates each result with a baselineState. Returns a
// BaselineSummary with bucket counts when comparison ran, or nil when
// it didn't.
func (s *AnalyzeService) applyBaseline(ctx context.Context, sarifLog *sarif.Log, baselineRef string) (*BaselineSummary, error) {
	sarif.EnsureAutomationDetails(sarifLog)
	if baselineRef == "" {
		return nil, nil
	}
	baselineLog, err := store.LoadBaseline(ctx, s.store, baselineRef)
	if err != nil {
		return nil, fmt.Errorf("loading baseline %q: %w", baselineRef, err)
	}
	sarif.CompareBaseline(sarifLog, baselineLog)

	summary := &BaselineSummary{Source: baselineRef}
	if len(sarifLog.Runs) > 0 {
		for _, r := range sarifLog.Runs[0].Results {
			switch r.BaselineState {
			case sarif.BaselineStateNew:
				summary.New++
			case sarif.BaselineStateUnchanged:
				summary.Unchanged++
			case sarif.BaselineStateAbsent:
				summary.Absent++
			}
		}
	}
	return summary, nil
}

// AnalyzeStream runs analysis progressively, emitting per-tier results on a channel.
// The error channel is for fatal errors only (invalid config, all providers unreachable).
// Tier-level failures are reported as TierResult with an Error field.
// The result channel receives exactly one value when the stream completes.
func (s *AnalyzeService) AnalyzeStream(ctx context.Context, req AnalyzeRequest) (<-chan TierResult, <-chan AnalyzeResult, <-chan error) {
	tierCh := make(chan TierResult, 10)
	resultCh := make(chan AnalyzeResult, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)    // runs 3rd (last)
		defer close(resultCh) // runs 2nd
		defer close(tierCh)   // runs 1st

		personaPrompt, err := buildPersonaPrompt(ctx, req.Config)
		if err != nil {
			errCh <- err
			return
		}

		ta := analyzer.NewTieredAnalyzer(s.clientFactory(req.Config.Provider), tieredOptions(req.Rules)...)
		progressive := ta.AnalyzeProgressive(ctx, req.Artifacts, req.Config.Policies, personaPrompt)

		// Aggregate TieredResults by tier for SSE events
		currentTier := ""
		var currentResults []sarif.Result
		var allResults []sarif.Result
		tierStart := time.Now()
		tierSeen := false

		for tr := range progressive {
			tierName := tr.Tier.String()

			// When tier changes, flush the previous tier's aggregated results
			if currentTier != "" && tierName != currentTier {
				tierCh <- TierResult{
					Tier:      currentTier,
					Results:   currentResults,
					ElapsedMs: time.Since(tierStart).Milliseconds(),
				}
				currentResults = nil
				tierStart = time.Now()
				tierSeen = false
			}
			currentTier = tierName
			tierSeen = true

			if tr.Error != nil {
				tierCh <- TierResult{
					Tier:      tierName,
					ElapsedMs: time.Since(tierStart).Milliseconds(),
					Error:     tr.Error.Error(),
				}
				tierSeen = false
				continue
			}

			currentResults = append(currentResults, tr.Results...)
			allResults = append(allResults, tr.Results...)
		}

		// Flush final tier (always emit if the tier was seen, even with no results)
		if currentTier != "" && tierSeen {
			tierCh <- TierResult{
				Tier:      currentTier,
				Results:   currentResults,
				ElapsedMs: time.Since(tierStart).Milliseconds(),
			}
		}

		// Store final SARIF
		sarifLog := sarif.Assemble(allResults, BuildDescriptors(req.Config.Policies, req.Rules), scopeFromArtifacts(req.Artifacts), req.Config.Persona)

		baselineSummary, baselineErr := s.applyBaseline(ctx, sarifLog, req.BaselineID)
		if baselineErr != nil {
			errCh <- baselineErr
			return
		}

		suppressedCount := applySuppressions(sarifLog, req.SuppressionDir)

		resultID, err := s.store.WriteSARIF(ctx, sarifLog)
		if err != nil {
			errCh <- fmt.Errorf("storing SARIF: %w", err)
			return
		}

		resultCh <- AnalyzeResult{
			ResultID:      resultID,
			TotalFindings: countFindings(sarifLog),
			Suppressed:    suppressedCount,
			Baseline:      baselineSummary,
		}
	}()

	return tierCh, resultCh, errCh
}

// buildPersonaPrompt resolves the persona prompt and, when StrictFilter
// is enabled, appends the appropriate applicability filter (prose vs
// code). Mirrors the CLI's persona handling so every entrypoint
// produces the same prompt.
func buildPersonaPrompt(ctx context.Context, cfg config.Config) (string, error) {
	prompt, err := analyzer.GetPersonaPrompt(ctx, cfg.Persona)
	if err != nil {
		return "", fmt.Errorf("getting persona prompt: %w", err)
	}
	if cfg.StrictFilter {
		if analyzer.IsProsePersona(cfg.Persona) {
			prompt += analyzer.ProseApplicabilityFilterPrompt
		} else {
			prompt += analyzer.ApplicabilityFilterPrompt
		}
	}
	return prompt, nil
}

func tieredOptions(loadedRules []rules.Rule) []analyzer.TieredAnalyzerOption {
	if len(loadedRules) == 0 {
		return nil
	}
	return []analyzer.TieredAnalyzerOption{analyzer.WithInstantPatterns(loadedRules)}
}

// applySuppressions loads .gavel/suppressions.yaml from rootDir and
// stamps matching results in sarifLog. A zero rootDir disables
// suppression handling. Returns how many results ended up suppressed.
func applySuppressions(sarifLog *sarif.Log, rootDir string) int {
	if rootDir == "" {
		return 0
	}
	supps, err := suppression.Load(rootDir)
	if err != nil {
		slog.Warn("failed to load suppressions", "err", err, "root", rootDir)
		return 0
	}
	suppression.Apply(supps, sarifLog)
	count := 0
	for _, run := range sarifLog.Runs {
		for _, r := range run.Results {
			if len(r.Suppressions) > 0 {
				count++
			}
		}
	}
	return count
}

func countFindings(sarifLog *sarif.Log) int {
	if len(sarifLog.Runs) == 0 {
		return 0
	}
	return len(sarifLog.Runs[0].Results)
}

// filterByLineRange keeps only results whose first location's StartLine
// falls within [start, end] inclusive. Results without a location are
// dropped.
func filterByLineRange(results []sarif.Result, start, end int) []sarif.Result {
	var out []sarif.Result
	for _, r := range results {
		if len(r.Locations) == 0 {
			continue
		}
		line := r.Locations[0].PhysicalLocation.Region.StartLine
		if line >= start && line <= end {
			out = append(out, r)
		}
	}
	return out
}

// windowedContent returns the substring of content covering
// [start-window, end+window] (clamped to the file bounds), along with
// the 1-indexed line where the window begins. Lines are split on "\n".
func windowedContent(content string, start, end, window int) (string, int) {
	lines := strings.Split(content, "\n")
	scopeStart := start - window
	if scopeStart < 1 {
		scopeStart = 1
	}
	scopeEnd := end + window
	if scopeEnd > len(lines) {
		scopeEnd = len(lines)
	}
	return strings.Join(lines[scopeStart-1:scopeEnd], "\n"), scopeStart
}

// BuildDescriptors assembles SARIF reportingDescriptors from both enabled
// policies and loaded rules. Rule descriptors carry help/helpUri populated
// from the rule's remediation, CWE, and reference metadata.
func BuildDescriptors(policies map[string]config.Policy, loadedRules []rules.Rule) []sarif.ReportingDescriptor {
	var descriptors []sarif.ReportingDescriptor
	for name, p := range policies {
		if p.Enabled {
			descriptors = append(descriptors, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}
	for _, r := range loadedRules {
		descriptors = append(descriptors, r.ToSARIFDescriptor())
	}
	return descriptors
}

// scopeFromArtifacts determines the input scope string from artifact kinds.
func scopeFromArtifacts(artifacts []input.Artifact) string {
	for _, a := range artifacts {
		if a.Kind == input.KindDiff {
			return "diff"
		}
	}
	return "directory"
}
