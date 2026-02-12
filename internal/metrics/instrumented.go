package metrics

import (
	"context"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// Finding mirrors analyzer.Finding to avoid import cycle
type Finding struct {
	RuleID         string
	Level          string
	Message        string
	FilePath       string
	StartLine      int
	EndLine        int
	Recommendation string
	Explanation    string
	Confidence     float64
}

// BAMLClient mirrors analyzer.BAMLClient interface
type BAMLClient interface {
	AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error)
}

// Analyzer interface for the core analyzer
type Analyzer interface {
	Analyze(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) ([]sarif.Result, error)
}

// InstrumentedClient wraps a BAMLClient with metrics recording
type InstrumentedClient struct {
	client   BAMLClient
	recorder *Recorder
}

// NewInstrumentedClient creates a new instrumented BAML client
func NewInstrumentedClient(client BAMLClient, recorder *Recorder) *InstrumentedClient {
	return &InstrumentedClient{
		client:   client,
		recorder: recorder,
	}
}

// AnalyzeCode wraps the underlying client's AnalyzeCode with metrics
func (c *InstrumentedClient) AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error) {
	builder := c.recorder.StartAnalysis(AnalysisTypeFull, TierComprehensive)
	builder.WithFile("", code) // Path set later at artifact level
	builder.MarkStarted()

	findings, err := c.client.AnalyzeCode(ctx, code, policies, personaPrompt, additionalContext)
	if err != nil {
		builder.CompleteWithError(err)
		return nil, err
	}

	// Estimate tokens (rough approximation)
	tokensIn := estimateTokens(code + policies + personaPrompt + additionalContext)
	tokensOut := estimateTokens(formatFindings(findings))

	builder.Complete(len(findings), tokensIn, tokensOut)
	return findings, nil
}

// InstrumentedAnalyzer wraps an Analyzer with metrics recording
type InstrumentedAnalyzer struct {
	analyzer  Analyzer
	collector *Collector
	recorder  *Recorder
}

// NewInstrumentedAnalyzer creates a new instrumented analyzer
func NewInstrumentedAnalyzer(analyzer Analyzer, collector *Collector, provider, model string) *InstrumentedAnalyzer {
	return &InstrumentedAnalyzer{
		analyzer:  analyzer,
		collector: collector,
		recorder:  NewRecorder(collector, provider, model),
	}
}

// Analyze wraps the underlying analyzer's Analyze with metrics
func (a *InstrumentedAnalyzer) Analyze(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy, personaPrompt string) ([]sarif.Result, error) {
	// Record overall analysis
	builder := a.recorder.StartAnalysis(AnalysisTypeFull, TierComprehensive)
	
	// Aggregate file stats
	var totalSize, totalLines int
	for _, art := range artifacts {
		totalSize += len(art.Content)
		for _, c := range art.Content {
			if c == '\n' {
				totalLines++
			}
		}
		totalLines++ // Count last line
	}

	enabledPolicies := 0
	for _, p := range policies {
		if p.Enabled {
			enabledPolicies++
		}
	}

	builder.event.FileSize = totalSize
	builder.event.LineCount = totalLines
	builder.event.ChunkCount = len(artifacts)
	builder.event.PolicyCount = enabledPolicies

	builder.MarkStarted()

	results, err := a.analyzer.Analyze(ctx, artifacts, policies, personaPrompt)
	if err != nil {
		builder.CompleteWithError(err)
		return nil, err
	}

	// Estimate tokens for the full analysis
	tokensIn := estimateTokens(personaPrompt)
	for _, art := range artifacts {
		tokensIn += estimateTokens(art.Content)
	}
	tokensOut := len(results) * 100 // Rough estimate

	builder.Complete(len(results), tokensIn, tokensOut)
	return results, nil
}

// GetCollector returns the underlying collector
func (a *InstrumentedAnalyzer) GetCollector() *Collector {
	return a.collector
}

// GetRecorder returns the underlying recorder
func (a *InstrumentedAnalyzer) GetRecorder() *Recorder {
	return a.recorder
}

// estimateTokens provides a rough token count estimate
// Using ~4 characters per token as a rough approximation
func estimateTokens(text string) int {
	return len(text) / 4
}

// formatFindings converts findings to a string for token estimation
func formatFindings(findings []Finding) string {
	result := ""
	for _, f := range findings {
		result += f.RuleID + f.Level + f.Message + f.FilePath + f.Recommendation + f.Explanation
	}
	return result
}
