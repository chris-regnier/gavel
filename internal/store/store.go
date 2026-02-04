package store

import (
	"context"

	"github.com/chris-regnier/gavel/internal/sarif"
)

type Verdict struct {
	Decision         string                 `json:"decision"`
	Reason           string                 `json:"reason"`
	RelevantFindings []sarif.Result         `json:"relevant_findings,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

type Store interface {
	WriteSARIF(ctx context.Context, doc *sarif.Log) (string, error)
	WriteVerdict(ctx context.Context, sarifID string, verdict *Verdict) error
	ReadSARIF(ctx context.Context, id string) (*sarif.Log, error)
	ReadVerdict(ctx context.Context, sarifID string) (*Verdict, error)
	List(ctx context.Context) ([]string, error)
}
