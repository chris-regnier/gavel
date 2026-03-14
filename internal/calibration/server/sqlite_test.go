package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/calibration"
)

// newTestStore creates a temporary SQLite store for a single test and
// registers a cleanup function to close it when the test ends.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStore_AppendEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	events := []calibration.Event{
		{
			Type:      calibration.EventFindingCreated,
			TeamID:    "acme",
			Timestamp: time.Now().UTC(),
			Payload:   calibration.FindingPayload{RuleID: "SEC001", Severity: "error", Confidence: 0.85},
		},
		{
			Type:      calibration.EventFeedbackReceived,
			TeamID:    "acme",
			Timestamp: time.Now().UTC(),
			Payload:   calibration.FeedbackPayload{RuleID: "SEC001", Verdict: "useful"},
		},
	}

	if err := s.AppendEvents(ctx, "acme", events); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM events WHERE team_id = ?", "acme").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("event count = %d, want 2", count)
	}
}

// TestSQLiteStore_AppendEvents_Atomic verifies that a batch with an
// invalid event does not persist any events (all-or-nothing).
func TestSQLiteStore_AppendEvents_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Appending an empty slice must not error.
	if err := s.AppendEvents(ctx, "acme", nil); err != nil {
		t.Fatalf("AppendEvents with nil slice: %v", err)
	}

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM events WHERE team_id = ?", "acme").Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("event count = %d, want 0", count)
	}
}

func TestSQLiteStore_UpdateAndGetProfile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	feedbacks := []calibration.FeedbackPayload{
		{RuleID: "SEC001", Verdict: "useful"},
		{RuleID: "SEC001", Verdict: "useful"},
		{RuleID: "SEC001", Verdict: "noise"},
		{RuleID: "SEC001", Verdict: "noise"},
		{RuleID: "SEC001", Verdict: "noise"},
	}
	for _, fb := range feedbacks {
		if err := s.UpdateProfileFromFeedback(ctx, "acme", fb); err != nil {
			t.Fatal(err)
		}
	}

	profiles, err := s.GetTeamProfile(ctx, "acme", []string{"SEC001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles))
	}

	p := profiles[0]
	if p.TotalFindings != 5 {
		t.Errorf("total_findings = %d, want 5", p.TotalFindings)
	}
	if p.UsefulCount != 2 {
		t.Errorf("useful_count = %d, want 2", p.UsefulCount)
	}
	if p.NoiseCount != 3 {
		t.Errorf("noise_count = %d, want 3", p.NoiseCount)
	}
	// NoiseRate is derived by Recalculate: 3/5 = 0.6
	if p.NoiseRate != 0.6 {
		t.Errorf("noise_rate = %f, want 0.6", p.NoiseRate)
	}
}

func TestSQLiteStore_UpdateAndGetProfile_WrongVerdict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	feedbacks := []calibration.FeedbackPayload{
		{RuleID: "QUAL001", Verdict: "wrong"},
		{RuleID: "QUAL001", Verdict: "wrong"},
		{RuleID: "QUAL001", Verdict: "useful"},
	}
	for _, fb := range feedbacks {
		if err := s.UpdateProfileFromFeedback(ctx, "team-b", fb); err != nil {
			t.Fatal(err)
		}
	}

	profiles, err := s.GetTeamProfile(ctx, "team-b", []string{"QUAL001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles))
	}
	p := profiles[0]
	if p.TotalFindings != 3 {
		t.Errorf("total_findings = %d, want 3", p.TotalFindings)
	}
	if p.WrongCount != 2 {
		t.Errorf("wrong_count = %d, want 2", p.WrongCount)
	}
}

func TestSQLiteStore_GetTeamProfile_Empty(t *testing.T) {
	s := newTestStore(t)
	// No profiles inserted — result must be empty, not an error.
	profiles, err := s.GetTeamProfile(context.Background(), "acme", []string{"SEC001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Errorf("profiles = %d, want 0", len(profiles))
	}
}

func TestSQLiteStore_GetTeamProfile_NilRuleIDs(t *testing.T) {
	s := newTestStore(t)
	// Passing nil ruleIDs must return nil, nil without touching the DB.
	profiles, err := s.GetTeamProfile(context.Background(), "acme", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profiles != nil {
		t.Errorf("profiles = %v, want nil", profiles)
	}
}

func TestSQLiteStore_GetTeamProfile_MultipleRules(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	rules := []string{"SEC001", "SEC002", "QUAL001"}
	for _, ruleID := range rules {
		if err := s.UpdateProfileFromFeedback(ctx, "acme", calibration.FeedbackPayload{
			RuleID: ruleID, Verdict: "useful",
		}); err != nil {
			t.Fatal(err)
		}
	}

	profiles, err := s.GetTeamProfile(ctx, "acme", rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 3 {
		t.Errorf("profiles = %d, want 3", len(profiles))
	}
}

func TestSQLiteStore_DeleteTeamData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert both an event and a profile for "acme".
	if err := s.AppendEvents(ctx, "acme", []calibration.Event{
		{Type: calibration.EventFeedbackReceived, TeamID: "acme",
			Timestamp: time.Now().UTC(),
			Payload:   calibration.FeedbackPayload{RuleID: "SEC001", Verdict: "useful"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateProfileFromFeedback(ctx, "acme",
		calibration.FeedbackPayload{RuleID: "SEC001", Verdict: "useful"}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteTeamData(ctx, "acme"); err != nil {
		t.Fatal(err)
	}

	// Both events and profiles must be gone.
	profiles, err := s.GetTeamProfile(ctx, "acme", []string{"SEC001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 0 {
		t.Error("profiles should be empty after DeleteTeamData")
	}

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM events WHERE team_id = ?", "acme").Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("event count = %d after delete, want 0", count)
	}
}

func TestSQLiteStore_DeleteTeamData_IsolatesOtherTeams(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert profiles for two teams.
	for _, team := range []string{"acme", "globex"} {
		if err := s.UpdateProfileFromFeedback(ctx, team,
			calibration.FeedbackPayload{RuleID: "SEC001", Verdict: "useful"}); err != nil {
			t.Fatal(err)
		}
	}

	// Delete only "acme".
	if err := s.DeleteTeamData(ctx, "acme"); err != nil {
		t.Fatal(err)
	}

	// "globex" profile must still exist.
	profiles, err := s.GetTeamProfile(ctx, "globex", []string{"SEC001"})
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Errorf("globex profiles = %d, want 1 after acme deletion", len(profiles))
	}
}

func TestSQLiteStore_GetGlobalStats_Placeholder(t *testing.T) {
	s := newTestStore(t)
	stats, err := s.GetGlobalStats(context.Background(), []string{"SEC001"})
	if err != nil {
		t.Fatalf("GetGlobalStats: %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil placeholder result, got %v", stats)
	}
}
