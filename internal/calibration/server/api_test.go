package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/calibration"
)

// Note: newTestStore is defined in sqlite_test.go and returns *SQLiteStore.

func TestHealthEndpoint(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, "test-key")
	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAuthMiddleware_MissingKey(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, "test-key")
	req := httptest.NewRequest(http.MethodGet, "/v1/calibration/acme", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, "test-key")
	req := httptest.NewRequest(http.MethodGet, "/v1/calibration/acme", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code == http.StatusUnauthorized {
		t.Error("should not be 401 with valid key")
	}
}

func TestIngestEvents(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, "test-key")
	body := `{"team_id":"acme","events":[{"type":"finding_created","team_id":"acme","timestamp":"2026-03-04T00:00:00Z","payload":{"rule_id":"SEC001","severity":"error","confidence":0.85}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/events/batch", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]int
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["queued"] != 1 {
		t.Errorf("queued = %d, want 1", resp["queued"])
	}
}

func TestIngestEvents_InvalidBody(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, "test-key")
	req := httptest.NewRequest(http.MethodPost, "/v1/events/batch", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestGetCalibration_WithProfile verifies that a team with a high-noise rule
// (≥70% noise rate with a non-zero mean noise confidence) receives a non-zero
// SuppressBelow threshold in the calibration response.
//
// The test seeds 8 feedback verdicts (6 noise, 2 useful) and then sets a
// non-zero mean_noise_conf directly on the materialized profile row so that
// RuleProfile.Recalculate can apply the suppression threshold logic.
func TestGetCalibration_WithProfile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed 8 feedback events: 6 noise + 2 useful → noise rate = 0.75, above
	// the 0.70 suppression threshold defined in RuleProfile.Recalculate.
	for i := 0; i < 8; i++ {
		verdict := "noise"
		if i < 2 {
			verdict = "useful"
		}
		if err := store.UpdateProfileFromFeedback(ctx, "acme", calibration.FeedbackPayload{
			RuleID:  "SEC001",
			Verdict: verdict,
		}); err != nil {
			t.Fatalf("UpdateProfileFromFeedback: %v", err)
		}
	}

	// Set mean_noise_conf to a non-zero value so that Recalculate will compute
	// a SuppressBelow threshold. UpdateProfileFromFeedback does not carry
	// per-finding confidence scores, so we seed it directly.
	_, err := store.db.ExecContext(ctx,
		"UPDATE team_rule_profiles SET mean_noise_conf = 0.5 WHERE team_id = ? AND rule_id = ?",
		"acme", "SEC001",
	)
	if err != nil {
		t.Fatalf("seed mean_noise_conf: %v", err)
	}

	srv := NewAPIServer(store, "test-key")
	req := httptest.NewRequest(http.MethodGet, "/v1/calibration/acme?rules=SEC001", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var resp calibration.CalibrationResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	th, ok := resp.TeamThresholds["SEC001"]
	if !ok {
		t.Fatal("missing SEC001 threshold")
	}
	if th.SuppressBelow == 0 {
		t.Error("expected non-zero suppress_below for high-noise rule")
	}
}

// TestIngestEvents_EmptyPayload verifies that requests with missing team_id or
// an empty events slice are rejected with 400 Bad Request.
func TestIngestEvents_EmptyPayload(t *testing.T) {
	store := newTestStore(t)
	srv := NewAPIServer(store, "test-key")

	cases := []struct {
		name string
		body string
	}{
		{"missing team_id", `{"events":[{"type":"finding_created"}]}`},
		{"empty events", `{"team_id":"acme","events":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/events/batch", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer test-key")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

// TestDeleteTeamData_API verifies the DELETE /v1/teams/{teamID}/data endpoint
// returns 204 No Content and that a subsequent calibration query returns no
// thresholds.
func TestDeleteTeamData_API(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Seed a profile so there is data to delete.
	if err := store.UpdateProfileFromFeedback(ctx, "acme", calibration.FeedbackPayload{
		RuleID:  "SEC001",
		Verdict: "noise",
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	srv := NewAPIServer(store, "test-key")

	// Issue the delete.
	req := httptest.NewRequest(http.MethodDelete, "/v1/teams/acme/data", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}

	// Confirm the calibration response is now empty.
	req2 := httptest.NewRequest(http.MethodGet, "/v1/calibration/acme?rules=SEC001", nil)
	req2.Header.Set("Authorization", "Bearer test-key")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d after delete", w2.Code)
	}
	var resp calibration.CalibrationResponse
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.TeamThresholds) != 0 {
		t.Errorf("expected empty thresholds after delete, got %v", resp.TeamThresholds)
	}
}
