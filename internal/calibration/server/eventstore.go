// Package server defines the server-side interfaces and types for the online
// calibration subsystem. Implementations of these interfaces handle event
// persistence, profile materialisation, and cross-org signal computation.
package server

import (
	"context"

	"github.com/chris-regnier/gavel/internal/calibration"
)

// EventStore persists calibration events and materialized profiles.
//
// All methods accept a context so that callers can enforce deadlines and
// propagate cancellation. Implementations must be safe for concurrent use.
type EventStore interface {
	// AppendEvents stores a batch of events for the given team. Events are
	// appended in the order provided; the store must not reorder them.
	// Returns an error if any event in the batch cannot be persisted — in that
	// case the implementation should treat the batch atomically (all-or-nothing)
	// where the underlying storage supports it.
	AppendEvents(ctx context.Context, teamID string, events []calibration.Event) error

	// GetTeamProfile returns the materialised calibration profile for each of
	// the requested ruleIDs belonging to teamID. Rules that have no profile yet
	// are omitted from the result slice rather than returned as zero values.
	GetTeamProfile(ctx context.Context, teamID string, ruleIDs []string) ([]calibration.RuleProfile, error)

	// GetGlobalStats returns anonymised cross-org calibration signals for each
	// of the requested ruleIDs. The returned map is keyed by rule ID. Rules
	// that have no cross-org data are omitted from the map.
	GetGlobalStats(ctx context.Context, ruleIDs []string) (map[string]calibration.CrossOrgSignal, error)

	// UpdateProfileFromFeedback incrementally updates the materialised
	// RuleProfile for the rule identified in fb.RuleID, belonging to teamID.
	// Implementations should apply the feedback without requiring a full replay
	// of all events, enabling low-latency profile updates on the hot path.
	UpdateProfileFromFeedback(ctx context.Context, teamID string, fb calibration.FeedbackPayload) error

	// DeleteTeamData removes all events and materialised profiles associated
	// with teamID. This method exists to satisfy GDPR right-to-erasure
	// obligations and must purge data completely from the backing store.
	DeleteTeamData(ctx context.Context, teamID string) error

	// Close releases any resources held by the store (connections, file
	// handles, background goroutines, etc.). After Close returns, no other
	// methods may be called.
	Close() error
}
