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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

var evalTracer = otel.Tracer("github.com/chris-regnier/gavel/internal/evaluator")

//go:embed default.rego
var defaultPolicy string

type Evaluator struct {
	query rego.PreparedEvalQuery
}

// NewEvaluator creates an evaluator. If policyDir is empty, uses the default policy.
// If policyDir is set, loads all .rego files from that directory (overriding default).
func NewEvaluator(ctx context.Context, policyDir string) (*Evaluator, error) {

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
	ctx, span := evalTracer.Start(ctx, "evaluate rego")
	defer span.End()

	data, err := json.Marshal(log)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	var input interface{}
	if err := json.Unmarshal(data, &input); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	results, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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

	span.SetAttributes(
		attribute.String("gavel.decision", decision),
		attribute.Int("gavel.finding_count", resultCount),
		attribute.Int("gavel.relevant_count", len(relevant)),
	)

	return &store.Verdict{
		Decision:         decision,
		Reason:           fmt.Sprintf("Decision: %s based on %d findings", decision, resultCount),
		RelevantFindings: relevant,
	}, nil
}
