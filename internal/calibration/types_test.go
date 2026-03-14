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
