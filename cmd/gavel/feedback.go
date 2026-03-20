package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/feedback"
)

var (
	flagFeedbackResult  string
	flagFeedbackOutput  string
	flagFeedbackFinding int
	flagFeedbackVerdict string
	flagFeedbackReason  string
)

func init() {
	feedbackCmd := &cobra.Command{
		Use:   "feedback",
		Short: "Provide feedback on analysis findings",
		Long:  "Mark findings as useful, noise, or wrong to improve future analysis quality.",
		RunE:  runFeedback,
	}

	feedbackCmd.Flags().StringVar(&flagFeedbackResult, "result", "", "Analysis result ID (required)")
	feedbackCmd.Flags().StringVar(&flagFeedbackOutput, "output", ".gavel/results", "Directory containing analysis results")
	feedbackCmd.Flags().IntVar(&flagFeedbackFinding, "finding", -1, "Finding index (required)")
	feedbackCmd.Flags().StringVar(&flagFeedbackVerdict, "verdict", "", "Verdict: useful, noise, or wrong (required)")
	feedbackCmd.Flags().StringVar(&flagFeedbackReason, "reason", "", "Optional explanation for feedback")

	feedbackCmd.MarkFlagRequired("result")
	feedbackCmd.MarkFlagRequired("verdict")

	rootCmd.AddCommand(feedbackCmd)
}

func runFeedback(cmd *cobra.Command, args []string) error {
	// Validate verdict
	var verdict feedback.Verdict
	switch flagFeedbackVerdict {
	case "useful":
		verdict = feedback.VerdictUseful
	case "noise":
		verdict = feedback.VerdictNoise
	case "wrong":
		verdict = feedback.VerdictWrong
	default:
		return fmt.Errorf("invalid verdict %q: must be useful, noise, or wrong", flagFeedbackVerdict)
	}

	if flagFeedbackFinding < 0 {
		return fmt.Errorf("--finding is required (finding index, 0-based)")
	}

	// Resolve result directory
	resultDir := filepath.Join(flagFeedbackOutput, flagFeedbackResult)

	entry := feedback.Entry{
		FindingIndex: flagFeedbackFinding,
		RuleID:       "", // Could be enriched by reading SARIF
		Verdict:      verdict,
		Reason:       flagFeedbackReason,
		Timestamp:    time.Now(),
	}

	if err := feedback.AddEntry(resultDir, flagFeedbackResult, entry); err != nil {
		return fmt.Errorf("add feedback: %w", err)
	}

	// Show current feedback summary
	fb, err := feedback.ReadFeedback(resultDir)
	if err != nil {
		return err
	}

	stats := feedback.ComputeStats(fb.Entries)
	fmt.Printf("Feedback recorded for result %s (finding #%d: %s)\n", flagFeedbackResult, flagFeedbackFinding, verdict)
	fmt.Printf("Total feedback: %d (useful: %d, noise: %d, wrong: %d)\n", stats.Total, stats.Useful, stats.Noise, stats.Wrong)

	return nil
}
