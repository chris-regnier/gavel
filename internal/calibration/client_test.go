package calibration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientUploadEvents(t *testing.T) {
	var gotTeamID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/batch" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		var body struct{ TeamID string `json:"team_id"` }
		json.NewDecoder(r.Body).Decode(&body)
		gotTeamID = body.TeamID
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", 5*time.Second)
	err := c.UploadEvents(context.Background(), "acme", []Event{
		{Type: EventFindingCreated, TeamID: "acme", Timestamp: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotTeamID != "acme" {
		t.Errorf("team = %q, want acme", gotTeamID)
	}
}

func TestClientGetCalibration(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/calibration/acme" {
			t.Errorf("path = %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode(CalibrationResponse{
			TeamThresholds: map[string]ThresholdOverride{"SEC001": {SuppressBelow: 0.45}},
		})
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", 5*time.Second)
	resp, err := c.GetCalibration(context.Background(), "acme", []string{"SEC001"}, "go")
	if err != nil {
		t.Fatal(err)
	}
	if resp.TeamThresholds["SEC001"].SuppressBelow != 0.45 {
		t.Errorf("threshold = %f", resp.TeamThresholds["SEC001"].SuppressBelow)
	}
}

func TestClientGetCalibration_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-key", 50*time.Millisecond)
	_, err := c.GetCalibration(context.Background(), "acme", nil, "go")
	if err == nil {
		t.Error("expected timeout error")
	}
}
