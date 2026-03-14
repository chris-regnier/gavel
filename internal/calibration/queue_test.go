package calibration

import (
	"path/filepath"
	"testing"
	"time"
)

func TestQueue_EnqueueAndDrain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending_events")
	q := NewLocalQueue(dir)
	events := []Event{
		{Type: EventFindingCreated, TeamID: "acme", Timestamp: time.Now()},
		{Type: EventFeedbackReceived, TeamID: "acme", Timestamp: time.Now()},
	}
	if err := q.Enqueue("acme", events); err != nil {
		t.Fatal(err)
	}
	batches, err := q.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("batches = %d, want 1", len(batches))
	}
	if len(batches[0].Events) != 2 {
		t.Errorf("events = %d, want 2", len(batches[0].Events))
	}
}

func TestQueue_DrainEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending_events")
	q := NewLocalQueue(dir)
	batches, err := q.Drain()
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 0 {
		t.Errorf("batches = %d, want 0", len(batches))
	}
}

func TestQueue_Remove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending_events")
	q := NewLocalQueue(dir)
	q.Enqueue("acme", []Event{{Type: EventFindingCreated, TeamID: "acme", Timestamp: time.Now()}})
	batches, _ := q.Drain()
	if len(batches) != 1 {
		t.Fatal("expected 1 batch")
	}
	if err := q.Remove(batches[0].ID); err != nil {
		t.Fatal(err)
	}
	batches, _ = q.Drain()
	if len(batches) != 0 {
		t.Error("batch should be removed")
	}
}
