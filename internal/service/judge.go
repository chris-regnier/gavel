// internal/service/judge.go
package service

import (
	"context"
	"fmt"

	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/store"
)

// JudgeService orchestrates Rego-based verdict evaluation.
type JudgeService struct {
	store store.Store
}

// NewJudgeService creates a JudgeService.
func NewJudgeService(s store.Store) *JudgeService {
	return &JudgeService{store: s}
}

// Judge evaluates a stored SARIF result with Rego policies and stores the verdict.
func (s *JudgeService) Judge(ctx context.Context, req JudgeRequest) (*store.Verdict, error) {
	sarifLog, err := s.store.ReadSARIF(ctx, req.ResultID)
	if err != nil {
		return nil, fmt.Errorf("reading SARIF %s: %w", req.ResultID, err)
	}

	eval, err := evaluator.NewEvaluator(ctx, req.RegoDir)
	if err != nil {
		return nil, fmt.Errorf("creating evaluator: %w", err)
	}

	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		return nil, fmt.Errorf("evaluating: %w", err)
	}

	if err := s.store.WriteVerdict(ctx, req.ResultID, verdict); err != nil {
		return nil, fmt.Errorf("storing verdict: %w", err)
	}

	return verdict, nil
}
