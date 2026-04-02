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
	store   store.Store
	regoDir string // server-default Rego policy directory
}

// NewJudgeService creates a JudgeService with an optional default Rego directory.
func NewJudgeService(s store.Store, regoDir string) *JudgeService {
	return &JudgeService{store: s, regoDir: regoDir}
}

// Judge evaluates a stored SARIF result with Rego policies and stores the verdict.
func (s *JudgeService) Judge(ctx context.Context, req JudgeRequest) (*store.Verdict, error) {
	sarifLog, err := s.store.ReadSARIF(ctx, req.ResultID)
	if err != nil {
		return nil, fmt.Errorf("reading SARIF %s: %w", req.ResultID, err)
	}

	regoDir := req.RegoDir
	if regoDir == "" {
		regoDir = s.regoDir
	}
	eval, err := evaluator.NewEvaluator(ctx, regoDir)
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
