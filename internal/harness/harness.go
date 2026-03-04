package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// Harness executes comparison experiments between variants
type Harness struct {
	gavelBinary string
	workDir     string
	outputDir   string

	// ConfigPath is the path to the base policies.yaml
	configPath string

	// Config is the loaded base configuration
	config *config.Config

	// RepoManager handles external repository cloning
	repoManager *RepositoryManager
}

// New creates a new Harness instance
func New(gavelBinary, workDir string) *Harness {
	return &Harness{
		gavelBinary: gavelBinary,
		workDir:     workDir,
		outputDir:   filepath.Join(workDir, ".gavel", "results"),
		configPath:  filepath.Join(workDir, ".gavel", "policies.yaml"),
		repoManager: NewRepositoryManager(filepath.Join(workDir, ".gavel", "cache")),
	}
}

// SetOutputDir overrides the default output directory
func (h *Harness) SetOutputDir(dir string) {
	h.outputDir = dir
}

// LoadConfig loads the base configuration
func (h *Harness) LoadConfig() error {
	cfg, err := config.LoadFromFile(h.configPath)
	if err != nil {
		return fmt.Errorf("loading config from %s: %w", h.configPath, err)
	}
	if cfg == nil {
		return fmt.Errorf("config file not found: %s", h.configPath)
	}
	h.config = cfg
	return nil
}

// Run executes the harness with the given configuration
func (h *Harness) Run(ctx context.Context, cfg *HarnessConfig, resultsPath string) ([]RunMetrics, error) {
	if h.config == nil {
		if err := h.LoadConfig(); err != nil {
			return nil, err
		}
	}

	// Create output directory
	if err := os.MkdirAll(h.outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Clone external repositories if configured
	if len(cfg.Repos) > 0 {
		slog.Info("Preparing external repositories", "count", len(cfg.Repos))
		if _, err := h.repoManager.Prepare(ctx, cfg.Repos); err != nil {
			return nil, fmt.Errorf("preparing repos: %w", err)
		}
	}

	// Resolve targets to actual paths
	paths, err := h.repoManager.ResolveTargets(cfg.Targets, cfg.Packages)
	if err != nil {
		return nil, fmt.Errorf("resolving targets: %w", err)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no targets to analyze (specify packages or targets)")
	}

	// Clear results file
	if err := os.WriteFile(resultsPath, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("clearing results file: %w", err)
	}

	var allMetrics []RunMetrics

	slog.Info("Starting harness run",
		"variants", len(cfg.Variants),
		"runs", cfg.Runs,
		"targets", len(paths))

	for _, variant := range cfg.Variants {
		for run := 1; run <= cfg.Runs; run++ {
			for _, targetPath := range paths {
				metrics, err := h.runVariant(ctx, variant, run, targetPath)
				if err != nil {
					slog.Error("Run failed",
						"variant", variant.Name,
						"run", run,
						"target", targetPath,
						"error", err)
					continue
				}

				// Append to results file
				if err := metrics.WriteJSONL(resultsPath); err != nil {
					return nil, fmt.Errorf("writing results: %w", err)
				}

				allMetrics = append(allMetrics, metrics)
			}
		}
	}

	slog.Info("Harness run complete",
		"total_runs", len(allMetrics),
		"results_path", resultsPath)

	return allMetrics, nil
}

// runVariant executes a single variant run
func (h *Harness) runVariant(ctx context.Context, variant VariantConfig, run int, targetPath string) (RunMetrics, error) {
	start := time.Now()

	// Merge variant config with base config
	mergedCfg := variant.MergeWithConfig(h.config)

	// Create variant-specific config file
	variantConfigPath := filepath.Join(h.workDir, ".gavel", fmt.Sprintf("policies.%s.yaml", variant.Name))
	if err := h.writeVariantConfig(variantConfigPath, mergedCfg); err != nil {
		return RunMetrics{}, fmt.Errorf("writing variant config: %w", err)
	}
	defer os.Remove(variantConfigPath)

	// Run gavel analyze
	cmd := exec.CommandContext(ctx, h.gavelBinary,
		"analyze",
		"--dir", targetPath,
		"--output", h.outputDir,
		"--policies", filepath.Dir(variantConfigPath),
	)

	cmd.Dir = h.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("Analyze command output", "output", string(output))
		return RunMetrics{}, fmt.Errorf("running analyze: %w", err)
	}

	// Parse the result ID from output
	resultID := parseResultID(output)
	if resultID == "" {
		return RunMetrics{}, fmt.Errorf("no result ID in output")
	}

	// Run judge to get decision
	decision := "unknown"
	judgeCmd := exec.CommandContext(ctx, h.gavelBinary, "judge", "--result", resultID)
	judgeCmd.Dir = h.workDir
	judgeOutput, err := judgeCmd.CombinedOutput()
	if err == nil {
		decision = parseDecision(judgeOutput)
	}

	// Extract metrics from SARIF
	sarifPath := filepath.Join(h.outputDir, resultID, "sarif.json")
	metrics, err := extractMetrics(sarifPath)
	if err != nil {
		return RunMetrics{}, fmt.Errorf("extracting metrics: %w", err)
	}

	metrics.Run = run
	metrics.Variant = variant.Name
	metrics.Target = targetPath
	metrics.Decision = decision
	metrics.ResultID = resultID
	metrics.Duration = time.Since(start).Milliseconds()

	return metrics, nil
}

// writeVariantConfig writes a merged config to a file
func (h *Harness) writeVariantConfig(path string, cfg *config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// parseResultID extracts the result ID from gavel analyze output
func parseResultID(output []byte) string {
	var summary struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(output, &summary); err != nil {
		return ""
	}
	return summary.ID
}

// parseDecision extracts the decision from judge output
func parseDecision(output []byte) string {
	// Try to parse as JSON first
	var result struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(output, &result); err == nil && result.Decision != "" {
		return result.Decision
	}

	// Try to find JSON object in mixed output
	dec := json.NewDecoder(bytes.NewReader(output))
	var obj map[string]interface{}
	if err := dec.Decode(&obj); err == nil {
		if d, ok := obj["decision"].(string); ok {
			return d
		}
	}

	return "unknown"
}

// extractMetrics reads metrics from a SARIF file
func extractMetrics(path string) (RunMetrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RunMetrics{}, fmt.Errorf("reading SARIF: %w", err)
	}

	var log sarif.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return RunMetrics{}, fmt.Errorf("parsing SARIF: %w", err)
	}

	metrics := RunMetrics{}

	if len(log.Runs) == 0 {
		return metrics, nil
	}

	results := log.Runs[0].Results
	metrics.Total = len(results)

	var confSum float64
	tiers := make(map[string]int)
	levels := make(map[string]int)

	for _, r := range results {
		levels[r.Level]++

		tier := "comprehensive"
		if t, ok := r.Properties["gavel/tier"].(string); ok {
			tier = t
		}
		tiers[tier]++

		conf := 0.0
		if c, ok := r.Properties["gavel/confidence"].(float64); ok {
			conf = c
			confSum += conf
		}

		// Count high-confidence errors
		if r.Level == "error" && conf > 0.8 {
			metrics.HighConfErrors++
		}
	}

	metrics.LLM = tiers["comprehensive"]
	metrics.Instant = tiers["instant"]
	metrics.Errors = levels["error"]
	metrics.Warnings = levels["warning"]
	metrics.Notes = levels["note"]
	metrics.Nones = levels["none"]

	if metrics.Total > 0 {
		metrics.AvgConfidence = confSum / float64(metrics.Total)
	}

	return metrics, nil
}
