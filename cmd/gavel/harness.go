package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/harness"
)

var (
	harnessRuns       int
	harnessOutput     string
	harnessSummary    string
	harnessBaseline   string
	harnessPackages   []string
	harnessConfigPath string
)

func init() {
	harnessCmd := &cobra.Command{
		Use:   "harness",
		Short: "Run comparison experiments between variants",
	}

	runCmd := &cobra.Command{
		Use:   "run <variants.yaml>",
		Short: "Run a harness experiment",
		Args:  cobra.ExactArgs(1),
		RunE:  runHarness,
	}

	runCmd.Flags().IntVarP(&harnessRuns, "runs", "n", 0, "Number of runs per variant (default: from config or 3)")
	runCmd.Flags().StringVarP(&harnessOutput, "output", "o", "", "Output JSONL file (default: <workdir>/experiment-results.jsonl)")
	runCmd.Flags().StringSliceVar(&harnessPackages, "packages", nil, "Packages to analyze (default: from config)")
	runCmd.Flags().StringVar(&harnessConfigPath, "config", ".gavel/policies.yaml", "Base config file path")

	summarizeCmd := &cobra.Command{
		Use:   "summarize <results.jsonl>",
		Short: "Summarize harness results",
		Args:  cobra.ExactArgs(1),
		RunE:  summarizeHarness,
	}

	summarizeCmd.Flags().StringVar(&harnessBaseline, "baseline", "", "Baseline variant name for delta calculations")
	summarizeCmd.Flags().StringVarP(&harnessSummary, "format", "f", "text", "Output format: text, json, yaml")

	harnessCmd.AddCommand(runCmd)
	harnessCmd.AddCommand(summarizeCmd)
	rootCmd.AddCommand(harnessCmd)
}

func runHarness(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	variantsPath := args[0]

	// Load harness config
	cfg, err := harness.LoadHarnessConfigFile(variantsPath)
	if err != nil {
		return fmt.Errorf("loading harness config: %w", err)
	}

	// Apply CLI overrides
	if harnessRuns > 0 {
		cfg.Runs = harnessRuns
	}
	if len(harnessPackages) > 0 {
		cfg.Packages = harnessPackages
	}

	// Validate configuration
	if len(cfg.Variants) == 0 {
		return fmt.Errorf("no variants defined in %s", variantsPath)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid harness configuration: %w", err)
	}

	// Determine output path - use unique filename if not specified
	outputPath := harnessOutput
	if outputPath == "" {
		outputPath = fmt.Sprintf("experiment-results-%d.jsonl", time.Now().Unix())
	}

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Find and validate gavel binary
	gavelBinary := os.Getenv("GAVEL_BINARY")
	if gavelBinary == "" {
		gavelBinary = "gavel"
		// Validate that gavel binary exists in PATH
		if _, err := exec.LookPath(gavelBinary); err != nil {
			return fmt.Errorf("gavel binary not found in PATH: %w (set GAVEL_BINARY env var to specify location)", err)
		}
	} else {
		// Validate explicit path
		if info, err := os.Stat(gavelBinary); err != nil {
			return fmt.Errorf("gavel binary not found at %s: %w", gavelBinary, err)
		} else if info.Mode()&0111 == 0 {
			return fmt.Errorf("gavel binary at %s is not executable", gavelBinary)
		}
	}

	// Create harness
	h := harness.New(gavelBinary, workDir)

	// Load base config
	if err := h.LoadConfig(); err != nil {
		return fmt.Errorf("loading base config: %w", err)
	}

	// Run
	results, err := h.Run(ctx, cfg, outputPath)
	if err != nil {
		return err
	}

	fmt.Printf("\n=== ALL RUNS COMPLETE ===\n")
	fmt.Printf("Results in: %s\n", outputPath)
	fmt.Printf("Total runs: %d\n", len(results))
	fmt.Printf("\nTo summarize: gavel harness summarize %s", outputPath)
	if harnessBaseline != "" {
		fmt.Printf(" --baseline %s", harnessBaseline)
	}
	fmt.Println()

	return nil
}

func summarizeHarness(cmd *cobra.Command, args []string) error {
	resultsPath := args[0]

	var summary *harness.Summary
	var err error

	if harnessBaseline != "" {
		summary, err = harness.SummarizeWithBaseline(resultsPath, harnessBaseline)
	} else {
		summary, err = harness.Summarize(resultsPath)
	}
	if err != nil {
		return err
	}

	switch harnessSummary {
	case "json":
		data, err := harness.WriteSummaryJSON(summary, "")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "yaml":
		data, err := harness.WriteSummaryYAML(summary, "")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	default:
		harness.PrintSummary(summary)
	}

	return nil
}
