package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/bench"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "gavel-bench",
		Short: "Gavel benchmark harness",
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run benchmark suite against corpus",
		RunE:  runBenchmark,
	}
	runCmd.Flags().String("corpus", "benchmarks/corpus", "Path to benchmark corpus")
	runCmd.Flags().String("output", "", "Output file for results (default: stdout)")
	runCmd.Flags().Int("runs", 3, "Number of iterations per case")
	runCmd.Flags().Int("line-tolerance", 5, "Line matching tolerance")
	runCmd.Flags().String("persona", "code-reviewer", "Persona to use")
	runCmd.Flags().String("policies", ".gavel", "Directory containing policies.yaml")

	compareCmd := &cobra.Command{
		Use:   "compare <baseline> <current>",
		Short: "Compare two benchmark result files",
		Args:  cobra.ExactArgs(2),
		RunE:  compareBenchmarks,
	}

	rootCmd.AddCommand(runCmd, compareCmd)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	corpusDir, _ := cmd.Flags().GetString("corpus")
	outputFile, _ := cmd.Flags().GetString("output")
	runs, _ := cmd.Flags().GetInt("runs")
	tolerance, _ := cmd.Flags().GetInt("line-tolerance")
	persona, _ := cmd.Flags().GetString("persona")
	policyDir, _ := cmd.Flags().GetString("policies")

	corpus, err := bench.LoadCorpus(corpusDir)
	if err != nil {
		return fmt.Errorf("load corpus: %w", err)
	}
	log.Printf("Loaded %d corpus cases", len(corpus.Cases))

	// Load config for provider setup
	machineConfig := os.ExpandEnv("$HOME/.config/gavel/policies.yaml")
	projectConfig := policyDir + "/policies.yaml"
	cfg, err := config.LoadTiered(machineConfig, projectConfig)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client := analyzer.NewBAMLLiveClient(cfg.Provider)

	runCfg := bench.RunConfig{
		Runs:          runs,
		LineTolerance: tolerance,
		Policies:      cfg.Policies,
		Persona:       persona,
	}

	result, err := bench.RunBenchmark(ctx, corpus, client, runCfg)
	if err != nil {
		return fmt.Errorf("run benchmark: %w", err)
	}

	result.Provider = cfg.Provider.Name
	result.Model = getModel(cfg.Provider)
	result.CorpusDir = corpusDir

	// Output results
	data, _ := json.MarshalIndent(result, "", "  ")
	if outputFile != "" {
		return os.WriteFile(outputFile, data, 0o644)
	}
	fmt.Println(string(data))
	return nil
}

func compareBenchmarks(cmd *cobra.Command, args []string) error {
	var baseline, current bench.BenchmarkResult

	for i, path := range args {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		target := &baseline
		if i == 1 {
			target = &current
		}
		if err := json.Unmarshal(data, target); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}

	fmt.Printf("Baseline (%s) vs Current (%s)\n\n", baseline.RunID, current.RunID)
	fmt.Printf("%-20s %10s %10s %10s\n", "Metric", "Baseline", "Current", "Delta")
	fmt.Printf("%-20s %10s %10s %10s\n", "------", "--------", "-------", "-----")

	printDelta := func(name string, b, c float64) {
		d := c - b
		sign := "+"
		if d < 0 {
			sign = ""
		}
		fmt.Printf("%-20s %10.3f %10.3f %s%10.3f\n", name, b, c, sign, d)
	}

	printDelta("Micro Precision", baseline.Aggregate.MicroPrecision, current.Aggregate.MicroPrecision)
	printDelta("Micro Recall", baseline.Aggregate.MicroRecall, current.Aggregate.MicroRecall)
	printDelta("Micro F1", baseline.Aggregate.MicroF1, current.Aggregate.MicroF1)
	printDelta("Hallucination Rate", baseline.Aggregate.HallucinRate, current.Aggregate.HallucinRate)

	return nil
}

func getModel(p config.ProviderConfig) string {
	switch p.Name {
	case "ollama":
		return p.Ollama.Model
	case "openrouter":
		return p.OpenRouter.Model
	case "anthropic":
		return p.Anthropic.Model
	case "bedrock":
		return p.Bedrock.Model
	case "openai":
		return p.OpenAI.Model
	default:
		return "unknown"
	}
}
