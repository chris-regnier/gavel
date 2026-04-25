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
	// May also be a path to a sarif.json file. Empty disables baseline
	// comparison.
	BaselineID string
	// SuppressionDir, if non-empty, points at the project root that
	// contains .gavel/suppressions.yaml. The service will load and
	// stamp matching suppressions on the SARIF results before storing.
	// Empty disables suppression handling entirely.
	SuppressionDir string
}

// ScopedAnalyzeRequest describes a scoped diff analysis: the instant
// tier runs against the full file artifact, the comprehensive tier
// runs against a content window around the changed lines, and findings
// outside the changed range are filtered out before assembly.
type ScopedAnalyzeRequest struct {
	// Artifact is the full file. Its Content drives the instant tier
	// directly; the comprehensive tier sees a windowed slice around
	// [ChangedStart, ChangedEnd].
	Artifact input.Artifact
	// ChangedStart and ChangedEnd are 1-indexed line numbers describing
	// the changed region. Findings whose StartLine falls outside this
	// range are filtered out.
	ChangedStart int
	ChangedEnd   int
	// ContextWindow is the number of lines on either side of the
	// changed region included in the comprehensive-tier scope. Zero
	// uses the default (10).
	ContextWindow int
	Config        config.Config
	Rules         []rules.Rule
	// BaselineID and SuppressionDir behave identically to AnalyzeRequest.
	BaselineID     string
	SuppressionDir string
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
	ResultID      string `json:"result_id"`
	TotalFindings int    `json:"total_findings"`
	// Suppressed counts results that ended up with at least one
	// SARIF suppression entry stamped by .gavel/suppressions.yaml.
	// Zero when SuppressionDir was empty.
	Suppressed int              `json:"suppressed,omitempty"`
	Baseline   *BaselineSummary `json:"baseline,omitempty"`
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
