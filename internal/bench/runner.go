package bench

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// JudgeConfig configures LLM-as-judge evaluation.
type JudgeConfig struct {
	Enabled bool
	Client  analyzer.BAMLClient
}

// RunConfig configures a benchmark run.
type RunConfig struct {
	Runs          int                      // Number of iterations for averaging
	LineTolerance int                      // Line matching tolerance
	Policies      map[string]config.Policy // Policies to use
	Persona       string                   // Persona prompt to use
	Judge         JudgeConfig              // Optional LLM-as-judge
}

// BenchmarkResult holds the complete results of a benchmark run.
type BenchmarkResult struct {
	RunID      string         `json:"run_id"`
	Timestamp  time.Time      `json:"timestamp"`
	Model      string         `json:"model,omitempty"`
	Provider   string         `json:"provider,omitempty"`
	CorpusDir  string         `json:"corpus_dir,omitempty"`
	Runs       int            `json:"runs"`
	Aggregate  AggregateScore `json:"aggregate"`
	PerCase    []CaseResult   `json:"per_case"`
	DurationMs int64          `json:"duration_ms"`
}

// CaseResult holds per-case results across all runs.
type CaseResult struct {
	CaseName     string       `json:"case_name"`
	Language     string       `json:"language,omitempty"`
	Category     string       `json:"category,omitempty"`
	Mean         CaseScore    `json:"mean"`
	StdDev       CaseScore    `json:"std_dev"`
	RunScores    []CaseScore  `json:"run_scores"`
	JudgeResult  *JudgeResult `json:"judge_result,omitempty"`
}

// RunBenchmark executes the benchmark suite against a corpus.
func RunBenchmark(ctx context.Context, corpus *Corpus, client analyzer.BAMLClient, cfg RunConfig) (*BenchmarkResult, error) {
	if cfg.Runs < 1 {
		cfg.Runs = 3
	}
	if cfg.LineTolerance < 1 {
		cfg.LineTolerance = 5
	}

	start := time.Now()
	result := &BenchmarkResult{
		RunID:     fmt.Sprintf("%s-bench", time.Now().Format("20060102T150405")),
		Timestamp: start,
		Runs:      cfg.Runs,
	}

	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, cfg.Persona)
	if err != nil {
		return nil, fmt.Errorf("get persona prompt: %w", err)
	}
	policiesText := analyzer.FormatPolicies(cfg.Policies)

	// Run each case N times
	for _, c := range corpus.Cases {
		caseResult := CaseResult{
			CaseName: c.Name,
			Language: c.Metadata.Language,
			Category: c.Metadata.Category,
		}

		var lastRunResults []sarif.Result
		for run := 0; run < cfg.Runs; run++ {
			// Run analysis
			findings, err := client.AnalyzeCode(ctx, c.SourceContent, policiesText, personaPrompt, "")
			if err != nil {
				return nil, fmt.Errorf("analyze case %s run %d: %w", c.Name, run, err)
			}

			// Convert findings to SARIF results
			results := findingsToResults(findings)
			lastRunResults = results

			// Score against expected
			score := ScoreCase(c, results, cfg.LineTolerance)
			caseResult.RunScores = append(caseResult.RunScores, score)
		}

		// Compute mean and stddev across runs
		caseResult.Mean = meanScore(caseResult.RunScores)
		caseResult.StdDev = stddevScore(caseResult.RunScores, caseResult.Mean)

		// Run LLM-as-judge on last run's results if enabled
		if cfg.Judge.Enabled && cfg.Judge.Client != nil && len(lastRunResults) > 0 {
			jr, err := JudgeCase(ctx, cfg.Judge.Client, c.Name, lastRunResults, c.SourceContent)
			if err != nil {
				return nil, fmt.Errorf("judge case %s: %w", c.Name, err)
			}
			caseResult.JudgeResult = jr
		}

		result.PerCase = append(result.PerCase, caseResult)
	}

	// Aggregate across all cases (using mean scores)
	var meanScores []CaseScore
	for _, cr := range result.PerCase {
		meanScores = append(meanScores, cr.Mean)
	}
	result.Aggregate = AggregateScores(meanScores)
	result.DurationMs = time.Since(start).Milliseconds()

	return result, nil
}

func findingsToResults(findings []analyzer.Finding) []sarif.Result {
	var results []sarif.Result
	for _, f := range findings {
		r := sarif.Result{
			RuleID:  f.RuleID,
			Level:   f.Level,
			Message: sarif.Message{Text: f.Message},
			Properties: map[string]interface{}{
				"gavel/confidence":     f.Confidence,
				"gavel/recommendation": f.Recommendation,
				"gavel/explanation":    f.Explanation,
			},
		}
		if f.StartLine > 0 {
			r.Locations = []sarif.Location{{
				PhysicalLocation: sarif.PhysicalLocation{
					ArtifactLocation: sarif.ArtifactLocation{URI: f.FilePath},
					Region:           sarif.Region{StartLine: f.StartLine, EndLine: f.EndLine},
				},
			}}
		}
		results = append(results, r)
	}
	return results
}

func meanScore(scores []CaseScore) CaseScore {
	if len(scores) == 0 {
		return CaseScore{}
	}
	n := float64(len(scores))
	var m CaseScore
	for _, s := range scores {
		m.TruePositives += s.TruePositives
		m.FalsePositives += s.FalsePositives
		m.FalseNegatives += s.FalseNegatives
		m.Hallucinations += s.Hallucinations
		m.Precision += s.Precision
		m.Recall += s.Recall
		m.F1 += s.F1
	}
	// Integer fields: use rounded mean
	m.TruePositives = int(math.Round(float64(m.TruePositives) / n))
	m.FalsePositives = int(math.Round(float64(m.FalsePositives) / n))
	m.FalseNegatives = int(math.Round(float64(m.FalseNegatives) / n))
	m.Hallucinations = int(math.Round(float64(m.Hallucinations) / n))
	m.Precision /= n
	m.Recall /= n
	m.F1 /= n
	return m
}

func stddevScore(scores []CaseScore, m CaseScore) CaseScore {
	if len(scores) < 2 {
		return CaseScore{}
	}
	n := float64(len(scores))
	var sumP2, sumR2, sumF12 float64
	for _, s := range scores {
		sumP2 += (s.Precision - m.Precision) * (s.Precision - m.Precision)
		sumR2 += (s.Recall - m.Recall) * (s.Recall - m.Recall)
		sumF12 += (s.F1 - m.F1) * (s.F1 - m.F1)
	}
	// Use sample stddev (Bessel's correction) since runs are a sample
	return CaseScore{
		Precision: math.Sqrt(sumP2 / (n - 1)),
		Recall:    math.Sqrt(sumR2 / (n - 1)),
		F1:        math.Sqrt(sumF12 / (n - 1)),
	}
}
