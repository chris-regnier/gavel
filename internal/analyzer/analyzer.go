package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/chris-regnier/gavel/internal/astcheck"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// BAMLClient defines the interface for the BAML-based code analysis client.
type BAMLClient interface {
	AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error)
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
	client            BAMLClient
	additionalContext string
}

// AnalyzerOption configures an Analyzer.
type AnalyzerOption func(*Analyzer)

// WithAdditionalContext sets the additional context passed to the LLM alongside each artifact.
// This is used to provide diff enrichment context (commit messages, full file contents,
// cross-file awareness) to reduce false positives during diff analysis.
func WithAdditionalContext(ctx string) AnalyzerOption {
	return func(a *Analyzer) {
		a.additionalContext = ctx
	}
}

// NewAnalyzer creates an Analyzer with the given BAMLClient and optional configuration.
func NewAnalyzer(client BAMLClient, opts ...AnalyzerOption) *Analyzer {
	a := &Analyzer{client: client}
	for _, opt := range opts {
		opt(a)
	}
	return a
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
// The personaPrompt provides the expert perspective for analysis (from GetPersonaPrompt).
// Additional context (set via WithAdditionalContext) is passed alongside each artifact
// to provide diff enrichment such as commit messages, full file contents, and cross-file awareness.
func (a *Analyzer) Analyze(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) ([]sarif.Result, error) {
	policyText := FormatPolicies(policies)
	if policyText == "" {
		return nil, nil
	}

	var allResults []sarif.Result

	for _, art := range artifacts {
		// Prepend the filename so the LLM knows which file it's analyzing.
		// Without this, models hallucinate conventional filenames (e.g. "handlers.go"
		// instead of the actual "server.go"), causing ~50% of findings to reference
		// nonexistent paths. See https://github.com/chris-regnier/gavel/issues/34.
		code := art.Content
		if art.Path != "" {
			code = fmt.Sprintf("// File: %s\n%s", art.Path, art.Content)
		}
		findings, err := a.client.AnalyzeCode(ctx, code, policyText, personaPrompt, a.additionalContext)
		if err != nil {
			return nil, fmt.Errorf("analyzing %s: %w", art.Path, err)
		}

		for _, f := range findings {
			path := f.FilePath
			if path == "" {
				path = art.Path
			}

			region := sarif.Region{
				StartLine: f.StartLine,
				EndLine:   f.EndLine,
				Snippet:   sarif.ExtractSnippet(art.Content, f.StartLine, f.EndLine),
			}

			physLoc := sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: path},
				Region:           region,
				ContextRegion:    sarif.ExtractContextRegion(art.Content, f.StartLine, f.EndLine),
			}

			loc := sarif.Location{
				PhysicalLocation: physLoc,
			}
			if ll := astcheck.ResolveLogicalLocation(art.Path, []byte(art.Content), f.StartLine); ll != nil {
				loc.LogicalLocations = []sarif.LogicalLocation{*ll}
			}

			allResults = append(allResults, sarif.Result{
				RuleID:  f.RuleID,
				Level:   f.Level,
				Message: sarif.Message{Text: f.Message},
				Locations: []sarif.Location{loc},
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
