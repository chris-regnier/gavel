// internal/service/types.go
package service

import (
	"context"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// AnalyzeRequest is the transport-agnostic input for analysis.
type AnalyzeRequest struct {
	Artifacts []input.Artifact
	Config    config.Config
	Rules     []rules.Rule
}

// TierResult represents results from a single analysis tier.
type TierResult struct {
	Tier      string         `json:"tier"`
	Results   []sarif.Result `json:"results"`
	ElapsedMs int64          `json:"elapsed_ms"`
	Error     string         `json:"error,omitempty"`
}

// AnalyzeResult is the final summary after all tiers complete.
type AnalyzeResult struct {
	ResultID      string `json:"result_id"`
	TotalFindings int    `json:"total_findings"`
}

// JudgeRequest is the transport-agnostic input for evaluation.
type JudgeRequest struct {
	ResultID string
	RegoDir  string
}

// SSEEvent is a typed SSE event for serialization.
type SSEEvent struct {
	Event string      `json:"event"` // "tier", "complete", "error"
	Data  interface{} `json:"data"`
}

// ResultLister provides read access to stored results.
type ResultLister interface {
	ReadSARIF(ctx context.Context, id string) (*sarif.Log, error)
	ReadVerdict(ctx context.Context, sarifID string) (*store.Verdict, error)
	List(ctx context.Context) ([]string, error)
}
