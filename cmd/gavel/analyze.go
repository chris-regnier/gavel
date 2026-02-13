package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

var (
	flagFiles     []string
	flagDiff      string
	flagDir       string
	flagOutput    string
	flagPolicyDir string
	flagRegoDir   string
)

func init() {
	analyzeCmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze code against policies",
		RunE:  runAnalyze,
	}

	analyzeCmd.Flags().StringSliceVar(&flagFiles, "files", nil, "Files to analyze")
	analyzeCmd.Flags().StringVar(&flagDiff, "diff", "", "Path to diff file (or - for stdin)")
	analyzeCmd.Flags().StringVar(&flagDir, "dir", "", "Directory to analyze")
	analyzeCmd.Flags().StringVar(&flagOutput, "output", ".gavel/results", "Output directory for results")
	analyzeCmd.Flags().StringVar(&flagPolicyDir, "policies", ".gavel", "Directory containing policies.yaml")
	analyzeCmd.Flags().StringVar(&flagRegoDir, "rego", ".gavel/rego", "Directory containing Rego policies")

	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	machineConfig := os.ExpandEnv("$HOME/.config/gavel/policies.yaml")
	projectConfig := flagPolicyDir + "/policies.yaml"
	cfg, err := config.LoadTiered(machineConfig, projectConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Override persona from CLI flag if provided
	if personaFlag, _ := cmd.Flags().GetString("persona"); personaFlag != "" {
		cfg.Persona = personaFlag
	}

	// Validate configuration (including persona)
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Load rules (default + user + project overrides)
	userRulesDir := os.ExpandEnv("$HOME/.config/gavel/rules")
	projectRulesDir := filepath.Join(flagPolicyDir, "rules")
	loadedRules, err := rules.LoadRules(userRulesDir, projectRulesDir)
	if err != nil {
		return fmt.Errorf("loading rules: %w", err)
	}
	_ = loadedRules

	// Get persona prompt from BAML
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, cfg.Persona)
	if err != nil {
		return fmt.Errorf("loading persona %s: %w", cfg.Persona, err)
	}

	// Read input
	h := input.NewHandler()
	var artifacts []input.Artifact
	var inputScope string

	switch {
	case len(flagFiles) > 0:
		artifacts, err = h.ReadFiles(flagFiles)
		inputScope = "files"
	case flagDiff != "":
		var diffContent string
		if flagDiff == "-" {
			data, readErr := os.ReadFile("/dev/stdin")
			if readErr != nil {
				return readErr
			}
			diffContent = string(data)
		} else {
			data, readErr := os.ReadFile(flagDiff)
			if readErr != nil {
				return readErr
			}
			diffContent = string(data)
		}
		artifacts, err = h.ReadDiff(diffContent)
		inputScope = "diff"
	case flagDir != "":
		artifacts, err = h.ReadDirectory(flagDir)
		inputScope = "directory"
	default:
		return fmt.Errorf("specify --files, --diff, or --dir")
	}
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	// Analyze with BAML
	client := analyzer.NewBAMLLiveClient(cfg.Provider)
	a := analyzer.NewAnalyzer(client)
	results, err := a.Analyze(ctx, artifacts, cfg.Policies, personaPrompt)
	if err != nil {
		return fmt.Errorf("analyzing: %w", err)
	}

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
	sarifLog := sarif.Assemble(results, rules, inputScope, cfg.Persona)

	// Store results
	fs := store.NewFileStore(flagOutput)
	id, err := fs.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return fmt.Errorf("storing SARIF: %w", err)
	}

	// Evaluate with Rego
	eval, err := evaluator.NewEvaluator(flagRegoDir)
	if err != nil {
		return fmt.Errorf("creating evaluator: %w", err)
	}

	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		return fmt.Errorf("evaluating: %w", err)
	}

	if err := fs.WriteVerdict(ctx, id, verdict); err != nil {
		return fmt.Errorf("storing verdict: %w", err)
	}

	// Output verdict
	out, _ := json.MarshalIndent(verdict, "", "  ")
	fmt.Println(string(out))

	return nil
}
