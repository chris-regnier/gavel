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
	// BaselineID, if non-empty, identifies a previously stored SARIF
	// result to compare against. Each finding in the new run will be
	// annotated with a baselineState (new, unchanged, or absent).
	// Empty disables baseline comparison.
	BaselineID string
}

// TierResult represents results from a single analysis tier.
type TierResult struct {
	Tier      string         `json:"tier"`
	Results   []sarif.Result `json:"results"`
	ElapsedMs int64          `json:"elapsed_ms"`
	Error     string         `json:"error,omitempty"`
}

// BaselineSummary breaks down how many results in an AnalyzeResult fell
// into each baselineState bucket. It is zero-valued when baseline
// comparison was not performed.
type BaselineSummary struct {
	Source    string `json:"source,omitempty"`
	New       int    `json:"new"`
	Unchanged int    `json:"unchanged"`
	Absent    int    `json:"absent"`
}

// AnalyzeResult is the final summary after all tiers complete.
type AnalyzeResult struct {
	ResultID      string           `json:"result_id"`
	TotalFindings int              `json:"total_findings"`
	Baseline      *BaselineSummary `json:"baseline,omitempty"`
}

// JudgeRequest is the transport-agnostic input for evaluation.
type JudgeRequest struct {
	ResultID string
	RegoDir  string
}

// ResultLister provides read access to stored results.
type ResultLister interface {
	ReadSARIF(ctx context.Context, id string) (*sarif.Log, error)
	ReadVerdict(ctx context.Context, sarifID string) (*store.Verdict, error)
	List(ctx context.Context) ([]string, error)
}
