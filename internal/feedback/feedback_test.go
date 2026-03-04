package feedback

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndReadFeedback(t *testing.T) {
	dir := t.TempDir()
	resultDir := filepath.Join(dir, "20260303-abc123")
	os.MkdirAll(resultDir, 0o755)

	fb := &Feedback{
		ResultID: "20260303-abc123",
		Entries: []Entry{
			{
				FindingIndex: 3,
				RuleID:       "SEC001",
				Verdict:      VerdictUseful,
				Reason:       "",
				Timestamp:    time.Now(),
			},
			{
				FindingIndex: 7,
				RuleID:       "QA003",
				Verdict:      VerdictNoise,
				Reason:       "style preference",
				Timestamp:    time.Now(),
			},
		},
	}

	err := WriteFeedback(resultDir, fb)
	if err != nil {
		t.Fatalf("WriteFeedback: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(resultDir, "feedback.json")); err != nil {
		t.Fatal("feedback.json not created")
	}

	loaded, err := ReadFeedback(resultDir)
	if err != nil {
		t.Fatalf("ReadFeedback: %v", err)
	}
	if loaded.ResultID != "20260303-abc123" {
		t.Errorf("ResultID = %q, want 20260303-abc123", loaded.ResultID)
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("Entries = %d, want 2", len(loaded.Entries))
	}
	if loaded.Entries[1].Verdict != VerdictNoise {
		t.Errorf("Verdict = %q, want noise", loaded.Entries[1].Verdict)
	}
}

func TestAddFeedbackEntry(t *testing.T) {
	dir := t.TempDir()
	resultDir := filepath.Join(dir, "test-result")
	os.MkdirAll(resultDir, 0o755)

	// Add first entry
	err := AddEntry(resultDir, "test-result", Entry{
		FindingIndex: 1,
		RuleID:       "SEC001",
		Verdict:      VerdictUseful,
		Timestamp:    time.Now(),
	})
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	// Add second entry
	err = AddEntry(resultDir, "test-result", Entry{
		FindingIndex: 2,
		RuleID:       "QA001",
		Verdict:      VerdictWrong,
		Reason:       "wrong file",
		Timestamp:    time.Now(),
	})
	if err != nil {
		t.Fatalf("AddEntry: %v", err)
	}

	fb, _ := ReadFeedback(resultDir)
	if len(fb.Entries) != 2 {
		t.Errorf("Entries = %d, want 2", len(fb.Entries))
	}
}
