package evaluator

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateSuppressedResultsExcluded(t *testing.T) {
	ctx := context.Background()
	eval, err := NewEvaluator(ctx, "")
	require.NoError(t, err)

	log := &sarif.Log{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarif.Run{{
			Tool: sarif.Tool{Driver: sarif.Driver{Name: "gavel", Version: "test"}},
			Results: []sarif.Result{
				{
					RuleID:  "S1001",
					Level:   "error",
					Message: sarif.Message{Text: "suppressed error"},
					Properties: map[string]interface{}{
						"gavel/confidence": 0.9,
					},
					Suppressions: []sarif.SARIFSuppression{
						{Kind: "external", Justification: "intentional"},
					},
				},
			},
		}},
	}

	verdict, err := eval.Evaluate(ctx, log)
	require.NoError(t, err)
	assert.Equal(t, "merge", verdict.Decision)
	assert.Empty(t, verdict.RelevantFindings)
	assert.Contains(t, verdict.Reason, "1 suppressed")
}

func TestEvaluateMixedSuppressedAndUnsuppressed(t *testing.T) {
	ctx := context.Background()
	eval, err := NewEvaluator(ctx, "")
	require.NoError(t, err)

	log := &sarif.Log{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarif.Run{{
			Tool: sarif.Tool{Driver: sarif.Driver{Name: "gavel", Version: "test"}},
			Results: []sarif.Result{
				{
					RuleID:  "S1001",
					Level:   "warning",
					Message: sarif.Message{Text: "active warning"},
				},
				{
					RuleID:  "S1002",
					Level:   "error",
					Message: sarif.Message{Text: "suppressed"},
					Suppressions: []sarif.SARIFSuppression{
						{Kind: "external", Justification: "fp"},
					},
				},
			},
		}},
	}

	verdict, err := eval.Evaluate(ctx, log)
	require.NoError(t, err)
	assert.Equal(t, "review", verdict.Decision)
	assert.Len(t, verdict.RelevantFindings, 1)
	assert.Equal(t, "S1001", verdict.RelevantFindings[0].RuleID)
	assert.Contains(t, verdict.Reason, "1 suppressed")
}
