package main

import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chris-regnier/gavel/internal/review"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Launch interactive PR review TUI",
	Long:  `Analyze code and launch an interactive terminal UI for reviewing findings.`,
	RunE:  runReview,
}

var (
	reviewDiff  string
	reviewFiles string
	reviewDir   string
)

func init() {
	reviewCmd.Flags().StringVar(&reviewDiff, "diff", "", "Path to unified diff (- for stdin)")
	reviewCmd.Flags().StringVar(&reviewFiles, "files", "", "Comma-separated list of files")
	reviewCmd.Flags().StringVar(&reviewDir, "dir", "", "Directory to analyze")

	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
	// For now, just load a SARIF file if provided as argument
	// TODO: Integrate with full analysis pipeline
	if len(args) == 0 {
		return fmt.Errorf("provide path to SARIF file for now (full analysis integration coming)")
	}

	sarifPath := args[0]
	log, err := loadSARIF(sarifPath)
	if err != nil {
		return fmt.Errorf("failed to load SARIF: %w", err)
	}

	model := review.NewReviewModel(log)
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
