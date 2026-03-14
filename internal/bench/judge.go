package bench

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// FindingVerdict is the LLM judge's assessment of a single finding.
type FindingVerdict struct {
	Score     int    `json:"score"`     // 1-5 quality score
	Label     string `json:"label"`     // "valid", "noise", "hallucination"
	Reasoning string `json:"reasoning"` // Why the judge scored it this way
}

// JudgeResult holds all verdicts for a benchmark case.
type JudgeResult struct {
	CaseName  string           `json:"case_name"`
	Verdicts  []FindingVerdict `json:"verdicts"`
	MeanScore float64          `json:"mean_score"`
	ValidRate float64          `json:"valid_rate"`
	NoiseRate float64          `json:"noise_rate"`
}

const judgePrompt = `You are an expert code reviewer evaluating the quality of automated code analysis findings.

For each finding, assess:
1. Is the finding describing a REAL issue in the code? (not speculative, not a style preference)
2. Is the severity level appropriate?
3. Is the recommendation actionable and correct?
4. Is the line reference accurate?

Respond with a JSON object:
{
  "score": <1-5>,
  "label": "<valid|noise|hallucination>",
  "reasoning": "<brief explanation>"
}

Score guide:
5 = Critical real issue, perfectly described
4 = Real issue, well described
3 = Minor real issue or slightly imprecise description
2 = Debatable/style issue presented as real problem
1 = Noise or hallucinated issue

Labels:
- valid: describes a real code issue
- noise: not a real issue (style preference, debatable, speculative)
- hallucination: references wrong file, wrong line, or nonexistent code`

// JudgeFinding uses an LLM to evaluate the quality of a single finding.
func JudgeFinding(ctx context.Context, client analyzer.BAMLClient, finding sarif.Result, sourceCode string) (*FindingVerdict, error) {
	prompt := fmt.Sprintf(`%s

SOURCE CODE:
%s

FINDING TO EVALUATE:
- Rule: %s
- Severity: %s
- Message: %s
- Confidence: %v
- Recommendation: %v
- Explanation: %v`,
		judgePrompt,
		sourceCode,
		finding.RuleID,
		finding.Level,
		finding.Message.Text,
		finding.Properties["gavel/confidence"],
		finding.Properties["gavel/recommendation"],
		finding.Properties["gavel/explanation"],
	)

	results, err := client.AnalyzeCode(ctx, prompt, "", "", "")
	if err != nil {
		return nil, fmt.Errorf("judge LLM call: %w", err)
	}

	if len(results) == 0 {
		return &FindingVerdict{Score: 1, Label: "noise", Reasoning: "no judge response"}, nil
	}

	var verdict FindingVerdict
	if err := json.Unmarshal([]byte(results[0].Message), &verdict); err != nil {
		// Fallback: treat the whole message as reasoning
		return &FindingVerdict{Score: 3, Label: "valid", Reasoning: results[0].Message}, nil
	}

	return &verdict, nil
}

// JudgeCase evaluates all findings for a benchmark case.
func JudgeCase(ctx context.Context, client analyzer.BAMLClient, caseName string, findings []sarif.Result, sourceCode string) (*JudgeResult, error) {
	result := &JudgeResult{CaseName: caseName}

	for _, f := range findings {
		v, err := JudgeFinding(ctx, client, f, sourceCode)
		if err != nil {
			return nil, fmt.Errorf("judge finding %s: %w", f.RuleID, err)
		}
		result.Verdicts = append(result.Verdicts, *v)
	}

	// Compute stats
	if len(result.Verdicts) > 0 {
		var totalScore int
		var valid, noise int
		for _, v := range result.Verdicts {
			totalScore += v.Score
			switch v.Label {
			case "valid":
				valid++
			case "noise":
				noise++
			}
		}
		n := float64(len(result.Verdicts))
		result.MeanScore = float64(totalScore) / n
		result.ValidRate = float64(valid) / n
		result.NoiseRate = float64(noise) / n
	}

	return result, nil
}
