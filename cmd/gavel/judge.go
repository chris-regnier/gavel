package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var judgeTracer = otel.Tracer("github.com/chris-regnier/gavel/cmd/gavel/judge")

var (
	flagJudgeResult    string
	flagJudgeOutput    string
	flagJudgeRegoDir   string
	flagJudgePolicyDir string
)

func init() {
	judgeCmd := &cobra.Command{
		Use:   "judge",
		Short: "Evaluate a SARIF analysis with Rego policies to produce a verdict",
		Long:  `Evaluate a previously generated SARIF analysis using Rego policies. By default evaluates the most recent analysis.`,
		RunE:  runJudge,
	}

	judgeCmd.Flags().StringVar(&flagJudgeResult, "result", "", "Analysis result ID to evaluate (default: most recent)")
	judgeCmd.Flags().StringVar(&flagJudgeOutput, "output", ".gavel/results", "Directory containing analysis results")
	judgeCmd.Flags().StringVar(&flagJudgeRegoDir, "rego", ".gavel/rego", "Directory containing Rego policies")
	judgeCmd.Flags().StringVar(&flagJudgePolicyDir, "policies", ".gavel", "Directory containing policies.yaml")

	rootCmd.AddCommand(judgeCmd)
}

func runJudge(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Load configuration (for telemetry settings)
	machineConfig := os.ExpandEnv("$HOME/.config/gavel/policies.yaml")
	projectConfig := flagJudgePolicyDir + "/policies.yaml"
	cfg, err := config.LoadTiered(machineConfig, projectConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Initialize telemetry
	shutdownTelemetry, err := telemetry.Init(ctx, cfg.Telemetry)
	if err != nil {
		return fmt.Errorf("initializing telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			log.Printf("Warning: telemetry shutdown error: %v", err)
		}
	}()

	fs := store.NewFileStore(flagJudgeOutput)

	// Resolve result ID: use provided or find most recent
	resultID := flagJudgeResult
	if resultID == "" {
		ids, err := fs.List(ctx)
		if err != nil {
			return fmt.Errorf("listing results: %w", err)
		}
		if len(ids) == 0 {
			return fmt.Errorf("no analysis results found in %s", flagJudgeOutput)
		}
		resultID = ids[0] // List returns newest first
	}

	ctx, span := judgeTracer.Start(ctx, "judge",
		trace.WithAttributes(
			attribute.String("gavel.result_id", resultID),
		),
	)
	defer span.End()

	// Load SARIF
	sarifLog, err := fs.ReadSARIF(ctx, resultID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("reading SARIF for %s: %w", resultID, err)
	}

	// Evaluate with Rego
	eval, err := evaluator.NewEvaluator(ctx, flagJudgeRegoDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("creating evaluator: %w", err)
	}

	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("evaluating: %w", err)
	}

	// Store verdict alongside the SARIF
	if err := fs.WriteVerdict(ctx, resultID, verdict); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("storing verdict: %w", err)
	}

	span.SetAttributes(
		attribute.String("gavel.decision", verdict.Decision),
	)

	// Output verdict
	out, _ := json.MarshalIndent(verdict, "", "  ")
	fmt.Println(string(out))

	return nil
}
