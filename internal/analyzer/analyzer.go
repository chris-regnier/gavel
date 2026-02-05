package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/chris-regnier/gavel/internal/config"
	gavelcontext "github.com/chris-regnier/gavel/internal/context"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// BAMLClient defines the interface for the BAML-based code analysis client.
type BAMLClient interface {
	AnalyzeCode(ctx context.Context, code string, policies string, additionalContext string) ([]Finding, error)
}

// Finding represents a single finding returned by the BAML analysis.
type Finding struct {
	RuleID         string  `json:"ruleId"`
	Level          string  `json:"level"`
	Message        string  `json:"message"`
	FilePath       string  `json:"filePath"`
	StartLine      int     `json:"startLine"`
	EndLine        int     `json:"endLine"`
	Recommendation string  `json:"recommendation"`
	Explanation    string  `json:"explanation"`
	Confidence     float64 `json:"confidence"`
}

// Analyzer orchestrates code analysis using a BAMLClient.
type Analyzer struct {
	client BAMLClient
}

// NewAnalyzer creates an Analyzer with the given BAMLClient.
func NewAnalyzer(client BAMLClient) *Analyzer {
	return &Analyzer{client: client}
}

// FormatPolicies formats enabled policies into a text block for the LLM prompt.
func FormatPolicies(policies map[string]config.Policy) string {
	var sb strings.Builder
	for name, p := range policies {
		if !p.Enabled {
			continue
		}
		fmt.Fprintf(&sb, "- %s [%s]: %s\n", name, p.Severity, p.Instruction)
	}
	return sb.String()
}

// Analyze runs the BAML client against each artifact and returns SARIF results.
// If contextLoader is provided, it will load additional context files for each artifact
// based on the policy's AdditionalContexts configuration.
func (a *Analyzer) Analyze(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, contextLoader *gavelcontext.Loader) ([]sarif.Result, error) {
	policyText := FormatPolicies(policies)
	if policyText == "" {
		return nil, nil
	}

	var allResults []sarif.Result

	for _, art := range artifacts {
		// Build additional context for this artifact
		var contextText string
		if contextLoader != nil {
			// Collect all context selectors from enabled policies
			var allSelectors []config.ContextSelector
			for _, p := range policies {
				if p.Enabled && len(p.AdditionalContexts) > 0 {
					allSelectors = append(allSelectors, p.AdditionalContexts...)
				}
			}
			
			// Load context files
			if len(allSelectors) > 0 {
				contextFiles, err := contextLoader.LoadForArtifact(art.Path, allSelectors)
				if err != nil {
					return nil, fmt.Errorf("loading context for %s: %w", art.Path, err)
				}
				contextText = gavelcontext.FormatContext(contextFiles)
			}
		}

		findings, err := a.client.AnalyzeCode(ctx, art.Content, policyText, contextText)
		if err != nil {
			return nil, fmt.Errorf("analyzing %s: %w", art.Path, err)
		}

		for _, f := range findings {
			path := f.FilePath
			if path == "" {
				path = art.Path
			}

			allResults = append(allResults, sarif.Result{
				RuleID:  f.RuleID,
				Level:   f.Level,
				Message: sarif.Message{Text: f.Message},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: path},
						Region:           sarif.Region{StartLine: f.StartLine, EndLine: f.EndLine},
					},
				}},
				Properties: map[string]interface{}{
					"gavel/recommendation": f.Recommendation,
					"gavel/explanation":    f.Explanation,
					"gavel/confidence":     f.Confidence,
				},
			})
		}
	}

	return allResults, nil
}
