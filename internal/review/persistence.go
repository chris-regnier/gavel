package review

import (
	"encoding/json"
	"os"
	"time"
)

// ReviewState represents persisted review state
type ReviewState struct {
	SarifID    string                   `json:"sarif_id"`
	ReviewedAt string                   `json:"reviewed_at"`
	Reviewer   string                   `json:"reviewer"`
	Findings   map[string]FindingReview `json:"findings"`
}

// FindingReview represents review status for a single finding
type FindingReview struct {
	Status  string `json:"status"`  // "accepted", "rejected", or ""
	Comment string `json:"comment"`
}

// SaveReviewState saves the review model state to a JSON file
func SaveReviewState(model *ReviewModel, sarifID string, filePath string) error {
	state := ReviewState{
		SarifID:    sarifID,
		ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Reviewer:   getUserEmail(),
		Findings:   make(map[string]FindingReview),
	}

	// Collect all reviewed findings
	for findingID := range model.accepted {
		state.Findings[findingID] = FindingReview{
			Status:  "accepted",
			Comment: model.comments[findingID],
		}
	}

	for findingID := range model.rejected {
		state.Findings[findingID] = FindingReview{
			Status:  "rejected",
			Comment: model.comments[findingID],
		}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// Use restrictive permissions - review state may contain sensitive information
	return os.WriteFile(filePath, data, 0600)
}

// LoadReviewState loads review state from a JSON file
func LoadReviewState(filePath string) (*ReviewState, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var state ReviewState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// getUserEmail returns the current user's email (stub for now)
func getUserEmail() string {
	// TODO: Get from git config or environment
	return os.Getenv("USER") + "@localhost"
}
