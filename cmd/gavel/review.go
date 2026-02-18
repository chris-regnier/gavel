package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chris-regnier/gavel/internal/review"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/spf13/cobra"
)

var (
	reviewResult string
	reviewOutput string
)

func init() {
	reviewCmd := &cobra.Command{
		Use:   "review [sarif-file]",
		Short: "Launch interactive PR review TUI",
		Long:  `Launch an interactive terminal UI for reviewing findings from a previous analysis. By default loads the most recent analysis.`,
		RunE:  runReview,
	}

	reviewCmd.Flags().StringVar(&reviewResult, "result", "", "Analysis result ID to review (default: most recent)")
	reviewCmd.Flags().StringVar(&reviewOutput, "output", ".gavel/results", "Directory containing analysis results")

	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	var sarifLog *sarif.Log
	var err error

	if len(args) > 0 {
		// Load SARIF from file path argument
		sarifLog, err = loadSARIF(args[0])
		if err != nil {
			return fmt.Errorf("failed to load SARIF: %w", err)
		}
	} else {
		// Load from store
		fs := store.NewFileStore(reviewOutput)

		resultID := reviewResult
		if resultID == "" {
			ids, err := fs.List(ctx)
			if err != nil {
				return fmt.Errorf("listing results: %w", err)
			}
			if len(ids) == 0 {
				return fmt.Errorf("no analysis results found in %s; run 'gavel analyze' first", reviewOutput)
			}
			resultID = ids[0]
		}

		sarifLog, err = fs.ReadSARIF(ctx, resultID)
		if err != nil {
			return fmt.Errorf("reading SARIF for %s: %w", resultID, err)
		}
	}

	// Launch TUI
	model := review.NewReviewModel(sarifLog)
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func loadSARIF(path string) (*sarif.Log, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var log sarif.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}

	return &log, nil
}
