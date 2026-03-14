package calibration

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventMarshalRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	e := Event{
		Type:      EventFindingCreated,
		TeamID:    "acme",
		Timestamp: now,
		Payload: FindingPayload{
			RuleID:     "SEC001",
			Severity:   "error",
			Confidence: 0.85,
			FileType:   "go",
			Message:    "SQL injection risk",
		},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != EventFindingCreated {
		t.Errorf("type = %q, want %q", got.Type, EventFindingCreated)
	}
	if got.TeamID != "acme" {
		t.Errorf("team = %q, want %q", got.TeamID, "acme")
	}
}

func TestEventTypeValidation(t *testing.T) {
	valid := []EventType{
		EventAnalysisCompleted,
		EventFindingCreated,
		EventFeedbackReceived,
		EventOutcomeObserved,
	}
	for _, et := range valid {
		if !et.Valid() {
			t.Errorf("%q should be valid", et)
		}
	}
	if EventType("bogus").Valid() {
		t.Error("bogus should be invalid")
	}
}

func TestTeamProfileNoiseRate(t *testing.T) {
	p := RuleProfile{TotalFindings: 100, UsefulCount: 60, NoiseCount: 30, WrongCount: 10}
	p.Recalculate()
	if p.NoiseRate != 0.3 {
		t.Errorf("noise_rate = %f, want 0.3", p.NoiseRate)
	}
	if p.ConfCalibration != 0.0 {
		t.Errorf("conf_cal = %f, want 0.0 (no confidence data)", p.ConfCalibration)
	}
}

func TestTeamProfileSuppressThreshold(t *testing.T) {
	p := RuleProfile{TotalFindings: 100, NoiseCount: 75, MeanNoiseConf: 0.5, MeanUsefulConf: 0.8}
	p.Recalculate()
	if p.SuppressBelow == 0 {
		t.Error("suppress_below should be set for high-noise rules")
	}
}

func TestCalibrationResponseJSON(t *testing.T) {
	resp := CalibrationResponse{
		TeamThresholds:  map[string]ThresholdOverride{"SEC001": {SuppressBelow: 0.45}},
		CrossOrgSignals: map[string]CrossOrgSignal{"SEC001": {GlobalNoiseRate: 0.23}},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var got CalibrationResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.TeamThresholds["SEC001"].SuppressBelow != 0.45 {
		t.Errorf("threshold = %f, want 0.45", got.TeamThresholds["SEC001"].SuppressBelow)
	}
}
