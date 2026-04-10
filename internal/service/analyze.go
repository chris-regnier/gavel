package service

import (
	"context"
	"fmt"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
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
	client := s.clientFactory(req.Config.Provider)

	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, req.Config.Persona)
	if err != nil {
		return nil, fmt.Errorf("getting persona prompt: %w", err)
	}

	opts := []analyzer.TieredAnalyzerOption{}
	if len(req.Rules) > 0 {
		opts = append(opts, analyzer.WithInstantPatterns(req.Rules))
	}

	ta := analyzer.NewTieredAnalyzer(client, opts...)
	results, err := ta.Analyze(ctx, req.Artifacts, req.Config.Policies, personaPrompt)
	if err != nil {
		return nil, fmt.Errorf("analyzing: %w", err)
	}

	sarifLog := sarif.Assemble(results, buildDescriptors(req.Config.Policies, req.Rules), scopeFromArtifacts(req.Artifacts), req.Config.Persona)

	resultID, err := s.store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return nil, fmt.Errorf("storing SARIF: %w", err)
	}

	return &AnalyzeResult{
		ResultID:      resultID,
		TotalFindings: len(results),
	}, nil
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

		client := s.clientFactory(req.Config.Provider)

		personaPrompt, err := analyzer.GetPersonaPrompt(ctx, req.Config.Persona)
		if err != nil {
			errCh <- fmt.Errorf("getting persona prompt: %w", err)
			return
		}

		opts := []analyzer.TieredAnalyzerOption{}
		if len(req.Rules) > 0 {
			opts = append(opts, analyzer.WithInstantPatterns(req.Rules))
		}

		ta := analyzer.NewTieredAnalyzer(client, opts...)
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
		sarifLog := sarif.Assemble(allResults, buildDescriptors(req.Config.Policies, req.Rules), scopeFromArtifacts(req.Artifacts), req.Config.Persona)
		resultID, err := s.store.WriteSARIF(ctx, sarifLog)
		if err != nil {
			errCh <- fmt.Errorf("storing SARIF: %w", err)
			return
		}

		resultCh <- AnalyzeResult{
			ResultID:      resultID,
			TotalFindings: len(allResults),
		}
	}()

	return tierCh, resultCh, errCh
}

// buildDescriptors assembles SARIF reportingDescriptors from both enabled
// policies and loaded rules. Rule descriptors carry help/helpUri populated
// from the rule's remediation, CWE, and reference metadata.
func buildDescriptors(policies map[string]config.Policy, loadedRules []rules.Rule) []sarif.ReportingDescriptor {
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
