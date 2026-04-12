package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/calibration"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/diffcontext"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/suppression"
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
	flagRulesDir    string
	flagCacheServer string
	flagBaseline    string
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
	analyzeCmd.Flags().StringVar(&flagRulesDir, "rules-dir", "", "Directory containing custom rule YAML files")
	analyzeCmd.Flags().StringVar(&flagCacheServer, "cache-server", "", "Remote cache server URL to upload results (e.g., https://gavel.company.com)")
	analyzeCmd.Flags().StringVar(&flagBaseline, "baseline", "", "Baseline SARIF to compare against (result ID from the store or a path to a sarif.json file). Each result gets a baselineState (new|unchanged|absent).")

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
			slog.Warn("telemetry shutdown error", "err", err)
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

	// Append applicability filter if enabled (default).
	// Prose personas get a writing-appropriate filter; code personas get the original.
	if cfg.StrictFilter {
		if analyzer.IsProsePersona(cfg.Persona) {
			personaPrompt += analyzer.ProseApplicabilityFilterPrompt
		} else {
			personaPrompt += analyzer.ApplicabilityFilterPrompt
		}
	}

	// Calibration: retrieve thresholds + few-shot examples
	var thresholdOverrides map[string]calibration.ThresholdOverride
	if cfg.Calibration.Enabled && cfg.Calibration.Retrieve.Enabled && cfg.Calibration.ServerURL != "" {
		apiKey := os.Getenv(cfg.Calibration.APIKeyEnv)
		if apiKey != "" {
			calClient := calibration.NewClient(
				cfg.Calibration.ServerURL, apiKey,
				time.Duration(cfg.Calibration.Retrieve.TimeoutMs)*time.Millisecond,
			)
			var ruleIDs []string
			for name, p := range cfg.Policies {
				if p.Enabled {
					ruleIDs = append(ruleIDs, name)
				}
			}
			calData, calErr := calClient.GetCalibration(ctx, "default", ruleIDs, "")
			if calErr != nil {
				slog.Warn("calibration retrieval failed, proceeding with defaults", "err", calErr)
			} else {
				thresholdOverrides = calData.TeamThresholds
				if cfg.Calibration.Retrieve.IncludeExamples && len(calData.FewShotExamples) > 0 {
					personaPrompt += calibration.FormatCalibrationExamples(calData.FewShotExamples)
				}
			}
		}
	}

	// Read input
	h := input.NewHandler()
	var artifacts []input.Artifact
	var inputScope string

	modeCount := 0
	if len(flagFiles) > 0 {
		modeCount++
	}
	if flagDiff != "" {
		modeCount++
	}
	if flagDir != "" {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("specify only one of --files, --diff, or --dir")
	}

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
	tieredOpts := []analyzer.TieredAnalyzerOption{analyzer.WithInstantPatterns(loadedRules)}

	// Build diff context to reduce false positives when analyzing diffs
	if inputScope == "diff" {
		repoDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		diffCtx := diffcontext.BuildDiffContext(artifacts, repoDir)
		if diffCtx != "" {
			tieredOpts = append(tieredOpts, analyzer.WithDiffContext(diffCtx))
			slog.Debug("diff context enrichment enabled", "context_size", len(diffCtx))
		}
	}

	ta := analyzer.NewTieredAnalyzer(client, tieredOpts...)
	results, err := ta.Analyze(ctx, artifacts, cfg.Policies, personaPrompt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("analyzing: %w", err)
	}

	descriptors := []sarif.ReportingDescriptor{}
	for name, p := range cfg.Policies {
		if p.Enabled {
			descriptors = append(descriptors, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}
	for _, r := range loadedRules {
		descriptors = append(descriptors, r.ToSARIFDescriptor())
	}

	// Assemble SARIF
	sarifLog := sarif.Assemble(results, descriptors, inputScope, cfg.Persona)

	// Stamp a stable automation guid so subsequent runs can reference this
	// one via baselineGuid.
	sarif.EnsureAutomationDetails(sarifLog)

	// Baseline comparison: annotate every result with new|unchanged|absent
	// relative to the baseline SARIF, if one was provided. This runs after
	// SARIF assembly (so content fingerprints are populated) and before
	// calibration/suppression (so they operate on a log that already
	// carries baselineState for downstream consumers to key off).
	baselineNew, baselineUnchanged, baselineAbsent := 0, 0, 0
	if flagBaseline != "" {
		baselineStore := store.NewFileStore(flagOutput)
		baselineLog, err := store.LoadBaseline(ctx, baselineStore, flagBaseline)
		if err != nil {
			return fmt.Errorf("loading baseline %q: %w", flagBaseline, err)
		}
		sarif.CompareBaseline(sarifLog, baselineLog)
		if len(sarifLog.Runs) > 0 {
			for _, r := range sarifLog.Runs[0].Results {
				switch r.BaselineState {
				case sarif.BaselineStateNew:
					baselineNew++
				case sarif.BaselineStateUnchanged:
					baselineUnchanged++
				case sarif.BaselineStateAbsent:
					baselineAbsent++
				}
			}
		}
	}

	// Calibration: apply threshold overrides
	if thresholdOverrides != nil && len(sarifLog.Runs) > 0 {
		suppressed := calibration.SuppressedResults(sarifLog.Runs[0].Results, thresholdOverrides)
		if len(suppressed) > 0 {
			slog.Info("calibration suppressed findings", "count", len(suppressed))
			sarifLog.Runs[0].Results = calibration.ApplyThresholds(sarifLog.Runs[0].Results, thresholdOverrides)
		}
	}

	// Apply suppressions
	suppressionRoot := filepath.Dir(flagPolicyDir)
	supps, err := suppression.Load(suppressionRoot)
	if err != nil {
		slog.Warn("failed to load suppressions", "err", err)
	}
	suppression.Apply(supps, sarifLog)

	suppressedCount := 0
	for _, run := range sarifLog.Runs {
		for _, r := range run.Results {
			if len(r.Suppressions) > 0 {
				suppressedCount++
			}
		}
	}

	// Store results
	fs := store.NewFileStore(flagOutput)
	id, err := fs.WriteSARIF(ctx, sarifLog)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("storing SARIF: %w", err)
	}

	// Calibration: upload events (non-blocking)
	if cfg.Calibration.Enabled && cfg.Calibration.Upload.Enabled && cfg.Calibration.ServerURL != "" {
		apiKey := os.Getenv(cfg.Calibration.APIKeyEnv)
		if apiKey != "" {
			go func() {
				calClient := calibration.NewClient(cfg.Calibration.ServerURL, apiKey, 10*time.Second)
				events := calibration.BuildEventsFromSARIF(sarifLog, id, cfg.Persona,
					cfg.Provider.Name, "", cfg.Calibration.ShareCode)
				if uploadErr := calClient.UploadEvents(context.Background(), "default", events); uploadErr != nil {
					slog.Warn("calibration upload failed, queuing locally", "err", uploadErr)
					q := calibration.NewLocalQueue(filepath.Join(flagPolicyDir, "pending_events"))
					if qErr := q.Enqueue("default", events); qErr != nil {
						slog.Error("failed to queue events locally", "err", qErr)
					}
				}
			}()
		}
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

	// Output analysis summary
	findingCount := 0
	if len(sarifLog.Runs) > 0 {
		findingCount = len(sarifLog.Runs[0].Results)
	}
	summary := map[string]interface{}{
		"id":         id,
		"findings":   findingCount,
		"scope":      inputScope,
		"persona":    cfg.Persona,
		"suppressed": suppressedCount,
	}
	if flagBaseline != "" {
		summary["baseline"] = map[string]interface{}{
			"source":    flagBaseline,
			"new":       baselineNew,
			"unchanged": baselineUnchanged,
			"absent":    baselineAbsent,
		}
	}
	out, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(out))

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
		slog.Warn("unrecognized provider for model lookup", "provider", cfg.Provider.Name)
		return cfg.Provider.Name
	}
}
