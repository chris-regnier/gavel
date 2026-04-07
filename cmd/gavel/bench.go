package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/bench"
	"github.com/chris-regnier/gavel/internal/config"
)

func init() {
	benchCmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmark models for code analysis quality, latency, and cost",
		RunE:  runBench,
	}

	benchCmd.Flags().Int("runs", 3, "Number of runs per model")
	benchCmd.Flags().Int("parallel", 4, "Number of models to benchmark in parallel")
	benchCmd.Flags().StringSlice("models", nil, "Comma-separated list of OpenRouter model IDs to benchmark (default: built-in set)")
	benchCmd.Flags().String("corpus", "", "Path to corpus directory (default: benchmarks/corpus)")
	benchCmd.Flags().String("output", ".gavel/bench", "Output directory for benchmark results")
	benchCmd.Flags().String("format", "json", "Output format: json or markdown")

	modelsCmd := &cobra.Command{
		Use:   "models",
		Short: "List available OpenRouter models with pricing",
		RunE:  runBenchModels,
	}
	modelsCmd.Flags().String("sort", "price", "Sort order: price or name")
	modelsCmd.Flags().Int("limit", 20, "Maximum number of models to display")

	benchCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(benchCmd)
}

func runBench(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Require OPENROUTER_API_KEY
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable is required")
	}

	// Parse flags
	runs, _ := cmd.Flags().GetInt("runs")
	parallel, _ := cmd.Flags().GetInt("parallel")
	modelFlags, _ := cmd.Flags().GetStringSlice("models")
	corpusDir, _ := cmd.Flags().GetString("corpus")
	outputDir, _ := cmd.Flags().GetString("output")

	// Load tiered config for policies and persona
	machineConfig := os.ExpandEnv("$HOME/.config/gavel/policies.yaml")
	projectConfig := ".gavel/policies.yaml"
	cfg, err := config.LoadTiered(machineConfig, projectConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Override persona from CLI flag if provided
	if personaFlag, _ := cmd.Flags().GetString("persona"); personaFlag != "" {
		cfg.Persona = personaFlag
	}

	// Resolve model list
	modelIDs := modelFlags
	if len(modelIDs) == 0 {
		modelIDs = bench.DefaultModels
	}

	slog.Info("fetching models from OpenRouter", "count", len(modelIDs))

	// Fetch and validate models from OpenRouter
	available, err := bench.FetchModels(ctx, apiKey)
	if err != nil {
		return fmt.Errorf("fetching models: %w", err)
	}

	validModels, err := bench.ValidateModelIDs(available, modelIDs)
	if err != nil {
		// Log warning but continue with valid models
		slog.Warn("some models not found on OpenRouter", "err", err)
		if len(validModels) == 0 {
			return fmt.Errorf("no valid models to benchmark")
		}
	}

	slog.Info("validated models", "count", len(validModels))

	// Load corpus
	if corpusDir == "" {
		corpusDir = "benchmarks/corpus"
	}
	corpus, err := bench.LoadCorpus(corpusDir)
	if err != nil {
		return fmt.Errorf("loading corpus from %s: %w", corpusDir, err)
	}
	slog.Info("loaded corpus", "cases", len(corpus.Cases))

	// Create clientFactory that makes a BAMLLiveClient for each OpenRouter model
	clientFactory := func(modelID string) analyzer.BAMLClient {
		providerCfg := config.ProviderConfig{
			Name: "openrouter",
			OpenRouter: config.OpenRouterConfig{
				Model: modelID,
			},
		}
		return analyzer.NewBAMLLiveClient(providerCfg)
	}

	// Run benchmark comparison
	compareCfg := bench.CompareConfig{
		Runs:     runs,
		Parallel: parallel,
		Models:   modelIDs,
		Policies: cfg.Policies,
		Persona:  cfg.Persona,
	}

	slog.Info("starting benchmark", "models", len(validModels), "runs", runs, "parallel", parallel)
	report, err := bench.RunComparison(ctx, corpus, validModels, clientFactory, compareCfg)
	if err != nil {
		return fmt.Errorf("running comparison: %w", err)
	}

	// Write JSON results to .gavel/bench/<timestamp>/results.json
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	resultDir := filepath.Join(outputDir, timestamp)
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	jsonPath := filepath.Join(resultDir, "results.json")
	jsonFile, err := os.Create(jsonPath)
	if err != nil {
		return fmt.Errorf("creating results file: %w", err)
	}
	defer jsonFile.Close()

	if err := bench.WriteJSON(jsonFile, report); err != nil {
		return fmt.Errorf("writing JSON results: %w", err)
	}
	slog.Info("wrote JSON results", "path", jsonPath)

	// Write markdown summary to docs/model-benchmarks.md
	mdPath := "docs/model-benchmarks.md"
	if err := os.MkdirAll(filepath.Dir(mdPath), 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}
	mdFile, err := os.Create(mdPath)
	if err != nil {
		return fmt.Errorf("creating markdown file: %w", err)
	}
	defer mdFile.Close()

	if err := bench.WriteMarkdown(mdFile, report); err != nil {
		return fmt.Errorf("writing markdown summary: %w", err)
	}
	slog.Info("wrote markdown summary", "path", mdPath)

	// Print summary to stdout
	fmt.Printf("\nBenchmark complete: %d models, %d corpus cases, %d runs each\n\n",
		len(report.Models), len(corpus.Cases), runs)
	fmt.Printf("%-40s  %6s  %9s  %8s  %10s  %11s\n",
		"MODEL", "F1", "PRECISION", "RECALL", "LATENCY(p50)", "COST/FILE")
	fmt.Printf("%s\n", strings.Repeat("-", 90))
	for _, m := range report.Models {
		fmt.Printf("%-40s  %6.2f  %9.2f  %8.2f  %10dms  $%10.4f\n",
			m.ModelID, m.Quality.F1, m.Quality.Precision, m.Quality.Recall,
			m.Latency.P50Ms, m.Cost.PerFileAvgUSD)
	}
	fmt.Printf("\nResults saved to: %s\n", jsonPath)
	fmt.Printf("Markdown summary: %s\n", mdPath)

	return nil
}

func runBenchModels(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Require OPENROUTER_API_KEY
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable is required")
	}

	sortBy, _ := cmd.Flags().GetString("sort")
	limit, _ := cmd.Flags().GetInt("limit")

	// Fetch models from OpenRouter
	models, err := bench.FetchModels(ctx, apiKey)
	if err != nil {
		return fmt.Errorf("fetching models: %w", err)
	}

	// Sort by requested field
	switch sortBy {
	case "name":
		sort.Slice(models, func(i, j int) bool {
			return models[i].ID < models[j].ID
		})
	default: // "price"
		bench.SortByPrice(models)
	}

	// Limit results
	if limit > 0 && len(models) > limit {
		models = models[:limit]
	}

	// Print table
	fmt.Printf("%-50s  %12s  %13s  %10s\n", "MODEL ID", "INPUT $/M", "OUTPUT $/M", "CONTEXT")
	fmt.Printf("%s\n", strings.Repeat("-", 92))
	for _, m := range models {
		fmt.Printf("%-50s  %12.4f  %13.4f  %10s\n",
			m.ID, m.InputPricePerM, m.OutputPricePerM, formatContext(m.ContextLength))
	}
	fmt.Printf("\n%d models listed\n", len(models))

	return nil
}

// formatContext converts a token count to a human-readable string like "1M" or "200K".
func formatContext(tokens int) string {
	if tokens == 0 {
		return "?"
	}
	if tokens >= 1_000_000 {
		m := tokens / 1_000_000
		r := (tokens % 1_000_000) / 100_000
		if r == 0 {
			return fmt.Sprintf("%dM", m)
		}
		return fmt.Sprintf("%d.%dM", m, r)
	}
	k := tokens / 1_000
	return fmt.Sprintf("%dK", k)
}
