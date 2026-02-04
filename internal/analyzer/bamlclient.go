package analyzer

import (
	"context"

	baml_client "github.com/chris-regnier/gavel/baml_client"
	"github.com/chris-regnier/gavel/baml_client/types"
)

// Ensure BAMLLiveClient satisfies the BAMLClient interface at compile time.
var _ BAMLClient = (*BAMLLiveClient)(nil)

// BAMLLiveClient wraps the generated BAML client to implement the BAMLClient interface.
type BAMLLiveClient struct{}

// NewBAMLLiveClient creates a new live BAML client that calls the LLM via OpenRouter.
func NewBAMLLiveClient() *BAMLLiveClient {
	return &BAMLLiveClient{}
}

// AnalyzeCode calls the generated BAML AnalyzeCode function and converts the results.
func (c *BAMLLiveClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]Finding, error) {
	results, err := baml_client.AnalyzeCode(ctx, code, policies)
	if err != nil {
		return nil, err
	}

	return convertFindings(results), nil
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
