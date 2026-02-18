package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/output"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var analyzeTracer = otel.Tracer("github.com/chris-regnier/gavel/cmd/gavel")

var (
	flagFiles       []string
	flagDiff        string
	flagDir         string
	flagOutput      string
	flagPolicyDir   string
	flagRegoDir     string
	flagRulesDir    string
	flagCacheServer string
	flagFormat      string
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
	analyzeCmd.Flags().StringVar(&flagRulesDir, "rules-dir", "", "Directory containing custom rule YAML files")
	analyzeCmd.Flags().StringVar(&flagCacheServer, "cache-server", "", "Remote cache server URL to upload results (e.g., https://gavel.company.com)")
	analyzeCmd.Flags().StringVarP(&flagFormat, "format", "f", "", "Output format: json, sarif, markdown, pretty (default: auto-detect)")

	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Load configuration
	machineConfig := os.ExpandEnv("$HOME/.config/gavel/policies.yaml")
	projectConfig := flagPolicyDir + "/policies.yaml"
	cfg, err := config.LoadTiered(machineConfig, projectConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Initialize telemetry (noop if disabled)
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
	if flagRulesDir != "" {
		projectRulesDir = flagRulesDir
	}
	loadedRules, err := rules.LoadRules(userRulesDir, projectRulesDir)
	if err != nil {
		return fmt.Errorf("loading rules: %w", err)
	}

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

	// Root span for the analysis pipeline
	ctx, span := analyzeTracer.Start(ctx, "analyze code",
		trace.WithAttributes(
			attribute.String("gavel.input_scope", inputScope),
			attribute.Int("gavel.artifact_count", len(artifacts)),
			attribute.String("gavel.persona", cfg.Persona),
			attribute.String("gavel.provider", cfg.Provider.Name),
		),
	)
	defer span.End()

	// Analyze with tiered analyzer (instant pattern matching + LLM)
	client := analyzer.NewBAMLLiveClient(cfg.Provider)
	ta := analyzer.NewTieredAnalyzer(client, analyzer.WithInstantPatterns(loadedRules))
	results, err := ta.Analyze(ctx, artifacts, cfg.Policies, personaPrompt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("storing SARIF: %w", err)
	}

	// Evaluate with Rego
	eval, err := evaluator.NewEvaluator(ctx, flagRegoDir)
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

	if err := fs.WriteVerdict(ctx, id, verdict); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("storing verdict: %w", err)
	}

	// Upload results to remote cache if configured
	remoteCacheURL := flagCacheServer
	if remoteCacheURL == "" && cfg.RemoteCache.Enabled && cfg.RemoteCache.Strategy.WriteToRemote {
		remoteCacheURL = cfg.RemoteCache.URL
	}

	if remoteCacheURL != "" {
		if err := uploadResultsToCache(ctx, cfg, remoteCacheURL, artifacts, results); err != nil {
			// Log but don't fail - local storage succeeded
			slog.Warn("cache upload failed", "err", err)
		}
	}

	// Format and output
	format := output.ResolveFormat(flagFormat, isatty.IsTerminal(os.Stdout.Fd()))
	formatter, err := output.NewFormatter(format)
	if err != nil {
		return err
	}
	data, err := formatter.Format(&output.AnalysisOutput{
		Verdict:  verdict,
		SARIFLog: sarifLog,
	})
	if err != nil {
		return fmt.Errorf("formatting output: %w", err)
	}
	os.Stdout.Write(data)

	return nil
}

// uploadResultsToCache uploads analysis results to the remote cache server
func uploadResultsToCache(ctx context.Context, cfg *config.Config, cacheURL string, artifacts []input.Artifact, results []sarif.Result) error {
	// Get auth token
	var opts []cache.RemoteCacheOption
	token, err := cfg.RemoteCache.GetRemoteCacheToken()
	if err != nil {
		return fmt.Errorf("getting cache token: %w", err)
	}
	if token != "" {
		opts = append(opts, cache.WithToken(token))
	}

	remoteCache := cache.NewRemoteCache(cacheURL, opts...)

	// Group results by file path
	resultsByFile := make(map[string][]sarif.Result)
	for _, r := range results {
		if len(r.Locations) > 0 {
			path := r.Locations[0].PhysicalLocation.ArtifactLocation.URI
			resultsByFile[path] = append(resultsByFile[path], r)
		}
	}

	// Build cache entries for each artifact
	for _, artifact := range artifacts {
		fileResults := resultsByFile[artifact.Path]

		// Compute file hash
		h := sha256.Sum256([]byte(artifact.Content))
		fileHash := hex.EncodeToString(h[:])

		// Build policy hashes
		policies := make(map[string]string)
		for name, p := range cfg.Policies {
			if p.Enabled {
				instrHash := sha256.Sum256([]byte(p.Instruction))
				policies[name] = hex.EncodeToString(instrHash[:])
			}
		}

		// Build cache key
		cacheKey := cache.CacheKey{
			FileHash:    fileHash,
			FilePath:    artifact.Path,
			Provider:    cfg.Provider.Name,
			Model:       getModelFromConfig(cfg),
			BAMLVersion: "1.0", // TODO: Get from BAML metadata
			Policies:    policies,
		}

		entry := &cache.CacheEntry{
			Key:       cacheKey,
			Results:   fileResults,
			Timestamp: time.Now().Unix(),
		}

		if err := remoteCache.Put(ctx, entry); err != nil {
			return fmt.Errorf("uploading results for %s: %w", artifact.Path, err)
		}
	}

	return nil
}

// getModelFromConfig extracts the model name from the provider config
func getModelFromConfig(cfg *config.Config) string {
	switch cfg.Provider.Name {
	case "ollama":
		return cfg.Provider.Ollama.Model
	case "openrouter":
		return cfg.Provider.OpenRouter.Model
	case "anthropic":
		return cfg.Provider.Anthropic.Model
	case "bedrock":
		return cfg.Provider.Bedrock.Model
	case "openai":
		return cfg.Provider.OpenAI.Model
	default:
		return "unknown"
	}
}
