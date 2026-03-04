package feedback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const feedbackFile = "feedback.json"

// Verdict represents a user's assessment of a finding.
type Verdict string

const (
	VerdictUseful Verdict = "useful"
	VerdictNoise  Verdict = "noise"
	VerdictWrong  Verdict = "wrong"
)

// Entry is a single piece of feedback on a finding.
type Entry struct {
	FindingIndex int       `json:"finding_index"`
	RuleID       string    `json:"rule_id"`
	Verdict      Verdict   `json:"verdict"`
	Reason       string    `json:"reason,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// Feedback holds all user feedback for a single analysis result.
type Feedback struct {
	ResultID string  `json:"result_id"`
	Entries  []Entry `json:"feedback"`
}

// WriteFeedback writes feedback to the result directory.
func WriteFeedback(resultDir string, fb *Feedback) error {
	data, err := json.MarshalIndent(fb, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal feedback: %w", err)
	}
	return os.WriteFile(filepath.Join(resultDir, feedbackFile), data, 0o644)
}

// ReadFeedback reads feedback from the result directory.
// Returns nil, nil if no feedback file exists.
func ReadFeedback(resultDir string) (*Feedback, error) {
	data, err := os.ReadFile(filepath.Join(resultDir, feedbackFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read feedback: %w", err)
	}
	var fb Feedback
	if err := json.Unmarshal(data, &fb); err != nil {
		return nil, fmt.Errorf("parse feedback: %w", err)
	}
	return &fb, nil
}

// AddEntry appends a feedback entry, creating the file if needed.
func AddEntry(resultDir, resultID string, entry Entry) error {
	fb, err := ReadFeedback(resultDir)
	if err != nil {
		return err
	}
	if fb == nil {
		fb = &Feedback{ResultID: resultID}
	}
	fb.Entries = append(fb.Entries, entry)
	return WriteFeedback(resultDir, fb)
}
