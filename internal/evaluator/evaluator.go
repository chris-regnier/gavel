package evaluator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

//go:embed default.rego
var defaultPolicy string

type Evaluator struct {
	query rego.PreparedEvalQuery
}

// NewEvaluator creates an evaluator. If policyDir is empty, uses the default policy.
// If policyDir is set, loads all .rego files from that directory (overriding default).
func NewEvaluator(policyDir string) (*Evaluator, error) {
	ctx := context.Background()

	modules := []func(*rego.Rego){
		rego.Query("data.gavel.gate.decision"),
		rego.Module("default.rego", defaultPolicy),
	}

	if policyDir != "" {
		entries, err := os.ReadDir(policyDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading policy dir: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".rego") {
				data, err := os.ReadFile(filepath.Join(policyDir, e.Name()))
				if err != nil {
					return nil, err
				}
				// Custom policies override the default
				modules = []func(*rego.Rego){
					rego.Query("data.gavel.gate.decision"),
					rego.Module(e.Name(), string(data)),
				}
			}
		}
	}

	query, err := rego.New(modules...).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("preparing rego query: %w", err)
	}

	return &Evaluator{query: query}, nil
}

func (e *Evaluator) Evaluate(ctx context.Context, log *sarif.Log) (*store.Verdict, error) {
	data, err := json.Marshal(log)
	if err != nil {
		return nil, err
	}
	var input interface{}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}

	results, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("evaluating rego: %w", err)
	}

	decision := "review"
	if len(results) > 0 && len(results[0].Expressions) > 0 {
		if d, ok := results[0].Expressions[0].Value.(string); ok {
			decision = d
		}
	}

	var relevant []sarif.Result
	if len(log.Runs) > 0 {
		for _, r := range log.Runs[0].Results {
			if decision == "reject" && r.Level == "error" {
				relevant = append(relevant, r)
			} else if decision == "review" && (r.Level == "warning" || r.Level == "error") {
				relevant = append(relevant, r)
			}
		}
	}

	resultCount := 0
	if len(log.Runs) > 0 {
		resultCount = len(log.Runs[0].Results)
	}

	return &store.Verdict{
		Decision:         decision,
		Reason:           fmt.Sprintf("Decision: %s based on %d findings", decision, resultCount),
		RelevantFindings: relevant,
	}, nil
}
