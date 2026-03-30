package service

import (
	"context"
	"fmt"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
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

	sarifLog := sarif.Assemble(results, policyRules(req.Config.Policies), scopeFromArtifacts(req.Artifacts), req.Config.Persona)

	resultID, err := s.store.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return nil, fmt.Errorf("storing SARIF: %w", err)
	}

	return &AnalyzeResult{
		ResultID:      resultID,
		TotalFindings: len(results),
	}, nil
}

// policyRules converts enabled policies to SARIF reporting descriptors.
func policyRules(policies map[string]config.Policy) []sarif.ReportingDescriptor {
	var rules []sarif.ReportingDescriptor
	for name, p := range policies {
		if p.Enabled {
			rules = append(rules, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}
	return rules
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
