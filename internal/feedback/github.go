package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitHubAlert represents a Code Scanning alert from the GitHub API.
type GitHubAlert struct {
	Number      int    `json:"number"`
	State       string `json:"state"` // "open", "dismissed", "fixed"
	DismissedBy struct {
		Login string `json:"login"`
	} `json:"dismissed_by"`
	DismissedReason string `json:"dismissed_reason"` // "false positive", "won't fix", "used in tests"
	Rule            struct {
		ID string `json:"id"`
	} `json:"rule"`
	MostRecentInstance struct {
		Ref     string `json:"ref"`
		Message struct {
			Text string `json:"text"`
		} `json:"message"`
		Location struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
		} `json:"location"`
	} `json:"most_recent_instance"`
}

// SyncGitHubFeedback pulls alert states from GitHub Code Scanning and
// converts them to feedback entries. Requires `gh` CLI to be installed
// and authenticated.
func SyncGitHubFeedback(repo, resultID, resultDir string) ([]Entry, error) {
	// Fetch alerts via gh CLI
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/code-scanning/alerts", repo),
		"--paginate",
		"-q", ".",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh api call failed: %w", err)
	}

	var alerts []GitHubAlert
	if err := json.Unmarshal(output, &alerts); err != nil {
		return nil, fmt.Errorf("parse alerts: %w", err)
	}

	var entries []Entry
	for _, alert := range alerts {
		verdict := mapAlertStateToVerdict(alert.State, alert.DismissedReason)
		if verdict == "" {
			continue // Skip open alerts — no feedback signal
		}

		entries = append(entries, Entry{
			FindingIndex: alert.Number,
			RuleID:       alert.Rule.ID,
			Verdict:      verdict,
			Reason:       formatGitHubReason(alert),
			Timestamp:    time.Now(),
		})
	}

	// Persist to feedback file
	for _, e := range entries {
		if err := AddEntry(resultDir, resultID, e); err != nil {
			return nil, fmt.Errorf("add entry: %w", err)
		}
	}

	return entries, nil
}

// mapAlertStateToVerdict maps GitHub alert states to feedback verdicts.
func mapAlertStateToVerdict(state, dismissedReason string) Verdict {
	switch state {
	case "fixed":
		return VerdictUseful
	case "dismissed":
		switch strings.TrimSpace(strings.ToLower(dismissedReason)) {
		case "false positive":
			return VerdictWrong
		case "won't fix", "used in tests":
			return VerdictNoise
		default:
			return VerdictNoise
		}
	default:
		return "" // "open" alerts don't provide feedback signal
	}
}

func formatGitHubReason(alert GitHubAlert) string {
	parts := []string{fmt.Sprintf("github alert #%d", alert.Number)}
	if alert.State == "dismissed" && alert.DismissedReason != "" {
		parts = append(parts, fmt.Sprintf("dismissed: %s", alert.DismissedReason))
	}
	if alert.DismissedBy.Login != "" {
		parts = append(parts, fmt.Sprintf("by %s", alert.DismissedBy.Login))
	}
	return strings.Join(parts, ", ")
}
