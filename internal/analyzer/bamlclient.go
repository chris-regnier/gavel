package analyzer

import (
	"context"
	"fmt"

	baml_client "github.com/chris-regnier/gavel/baml_client"
	"github.com/chris-regnier/gavel/baml_client/types"
	"github.com/chris-regnier/gavel/internal/config"
)

// Ensure BAMLLiveClient satisfies the BAMLClient interface at compile time.
var _ BAMLClient = (*BAMLLiveClient)(nil)

// BAMLLiveClient wraps the generated BAML client to implement the BAMLClient interface.
type BAMLLiveClient struct {
	providerConfig config.ProviderConfig
}

// NewBAMLLiveClient creates a new live BAML client that calls the LLM via configured provider.
func NewBAMLLiveClient(cfg config.ProviderConfig) *BAMLLiveClient {
	return &BAMLLiveClient{
		providerConfig: cfg,
	}
}

// AnalyzeCode calls the appropriate BAML client based on provider config.
func (c *BAMLLiveClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]Finding, error) {
	var results []types.Finding
	var err error

	switch c.providerConfig.Name {
	case "ollama":
		results, err = c.analyzeWithOllama(ctx, code, policies)
	case "openrouter":
		results, err = c.analyzeWithOpenRouter(ctx, code, policies)
	default:
		return nil, fmt.Errorf("unknown provider: %s", c.providerConfig.Name)
	}

	if err != nil {
		return nil, fmt.Errorf("analysis failed with %s: %w", c.providerConfig.Name, err)
	}

	return convertFindings(results), nil
}

func (c *BAMLLiveClient) analyzeWithOllama(ctx context.Context, code string, policies string) ([]types.Finding, error) {
	// Use WithClient to select the Ollama client at runtime
	return baml_client.AnalyzeCode(ctx, code, policies, baml_client.WithClient("Ollama"))
}

func (c *BAMLLiveClient) analyzeWithOpenRouter(ctx context.Context, code string, policies string) ([]types.Finding, error) {
	// Use WithClient to select the OpenRouter client at runtime
	return baml_client.AnalyzeCode(ctx, code, policies, baml_client.WithClient("OpenRouter"))
}

func convertFindings(bamlFindings []types.Finding) []Finding {
	findings := make([]Finding, len(bamlFindings))
	for i, f := range bamlFindings {
		findings[i] = Finding{
			RuleID:         f.RuleId,
			Level:          f.Level,
			Message:        f.Message,
			FilePath:       f.FilePath,
			StartLine:      int(f.StartLine),
			EndLine:        int(f.EndLine),
			Recommendation: f.Recommendation,
			Explanation:    f.Explanation,
			Confidence:     f.Confidence,
		}
	}
	return findings
}
