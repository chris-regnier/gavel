package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
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
	ctx := context.Background()
	var log *sarif.Log
	var err error

	// If SARIF file provided as argument, load it directly
	if len(args) > 0 {
		log, err = loadSARIF(args[0])
		if err != nil {
			return fmt.Errorf("failed to load SARIF: %w", err)
		}
	} else {
		// Run full analysis pipeline
		log, err = runAnalysisForReview(ctx)
		if err != nil {
			return fmt.Errorf("failed to analyze: %w", err)
		}
	}

	// Launch TUI
	model := review.NewReviewModel(log)
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

// runAnalysisForReview runs the analysis pipeline and returns SARIF log
func runAnalysisForReview(ctx context.Context) (*sarif.Log, error) {
	// Load configuration
	machineConfig := os.ExpandEnv("$HOME/.config/gavel/policies.yaml")
	projectConfig := ".gavel/policies.yaml"
	cfg, err := config.LoadTiered(machineConfig, projectConfig)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Read input based on flags
	h := input.NewHandler()
	var artifacts []input.Artifact
	var inputScope string

	switch {
	case len(reviewFiles) > 0:
		files := strings.Split(reviewFiles, ",")
		artifacts, err = h.ReadFiles(files)
		inputScope = "files"
	case reviewDiff != "":
		var diffContent string
		if reviewDiff == "-" {
			data, readErr := os.ReadFile("/dev/stdin")
			if readErr != nil {
				return nil, readErr
			}
			diffContent = string(data)
		} else {
			data, readErr := os.ReadFile(reviewDiff)
			if readErr != nil {
				return nil, readErr
			}
			diffContent = string(data)
		}
		artifacts, err = h.ReadDiff(diffContent)
		inputScope = "diff"
	case reviewDir != "":
		artifacts, err = h.ReadDirectory(reviewDir)
		inputScope = "directory"
	default:
		return nil, fmt.Errorf("specify --files, --diff, or --dir")
	}
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	// Analyze with BAML
	client := analyzer.NewBAMLLiveClient(cfg.Provider)
	a := analyzer.NewAnalyzer(client)
	results, err := a.Analyze(ctx, artifacts, cfg.Policies, cfg.Persona)
	if err != nil {
		return nil, fmt.Errorf("analyzing: %w", err)
	}

	// Build rules list for SARIF
	rules := []sarif.ReportingDescriptor{}
	for name, p := range cfg.Policies {
		if p.Enabled {
			rules = append(rules, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}

	// Assemble SARIF
	log := sarif.Assemble(results, rules, inputScope, cfg.Persona)

	return log, nil
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
