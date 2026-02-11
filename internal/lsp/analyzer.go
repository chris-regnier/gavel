// internal/lsp/analyzer.go
package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// AnalyzerWrapper wraps the BAML analyzer with caching
type AnalyzerWrapper struct {
	client analyzer.BAMLClient
	cfg    *config.Config
	cache  cache.CacheManager
}

// NewAnalyzerWrapper creates a new analyzer wrapper
func NewAnalyzerWrapper(client analyzer.BAMLClient, cfg *config.Config) *AnalyzerWrapper {
	return &AnalyzerWrapper{
		client: client,
		cfg:    cfg,
	}
}

// WithCache sets the cache manager and returns the wrapper for chaining
func (w *AnalyzerWrapper) WithCache(c cache.CacheManager) *AnalyzerWrapper {
	w.cache = c
	return w
}

// Analyze runs analysis on a file with optional caching
func (w *AnalyzerWrapper) Analyze(ctx context.Context, path, content string) ([]sarif.Result, error) {
	// Build cache key
	cacheKey := w.buildCacheKey(path, content)

	// Try cache if available
	if w.cache != nil {
		if entry, err := w.cache.Get(ctx, cacheKey); err == nil {
			// Cache hit
			return entry.Results, nil
		}
	}

	// Cache miss or no cache - run analysis
	policyText := w.formatPolicies()
	if policyText == "" {
		// No enabled policies
		return []sarif.Result{}, nil
	}

	// Get persona prompt
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, w.cfg.Persona)
	if err != nil {
		return nil, fmt.Errorf("getting persona prompt: %w", err)
	}

	findings, err := w.client.AnalyzeCode(ctx, content, policyText, personaPrompt, "")
	if err != nil {
		return nil, fmt.Errorf("analyzing %s: %w", path, err)
	}

	// Convert findings to SARIF results
	results := make([]sarif.Result, 0, len(findings))
	for _, f := range findings {
		results = append(results, w.findingToResult(f, path))
	}

	// Store in cache if available
	if w.cache != nil {
		entry := &cache.CacheEntry{
			Key:       cacheKey,
			Results:   results,
			Timestamp: time.Now().Unix(),
		}
		// Ignore cache write errors - analysis succeeded
		_ = w.cache.Put(ctx, entry)
	}

	return results, nil
}

// buildCacheKey computes a cache key from file content and config
func (w *AnalyzerWrapper) buildCacheKey(path, content string) cache.CacheKey {
	// Hash file content
	h := sha256.Sum256([]byte(content))
	fileHash := hex.EncodeToString(h[:])

	// Extract provider and model
	provider := w.cfg.Provider.Name
	model := w.getModel()

	// Hash enabled policies
	policies := make(map[string]string)
	for name, p := range w.cfg.Policies {
		if p.Enabled {
			// Hash the instruction to detect changes
			instrHash := sha256.Sum256([]byte(p.Instruction))
			policies[name] = hex.EncodeToString(instrHash[:])
		}
	}

	return cache.CacheKey{
		FileHash:    fileHash,
		FilePath:    path,
		Provider:    provider,
		Model:       model,
		BAMLVersion: "1.0", // TODO: Get from BAML metadata
		Policies:    policies,
	}
}

// getModel returns the model name from the config
func (w *AnalyzerWrapper) getModel() string {
	switch w.cfg.Provider.Name {
	case "ollama":
		return w.cfg.Provider.Ollama.Model
	case "openrouter":
		return w.cfg.Provider.OpenRouter.Model
	case "anthropic":
		return w.cfg.Provider.Anthropic.Model
	case "bedrock":
		return w.cfg.Provider.Bedrock.Model
	case "openai":
		return w.cfg.Provider.OpenAI.Model
	default:
		return "unknown"
	}
}

// formatPolicies formats enabled policies for BAML prompt
func (w *AnalyzerWrapper) formatPolicies() string {
	var sb strings.Builder
	for name, p := range w.cfg.Policies {
		if !p.Enabled {
			continue
		}
		fmt.Fprintf(&sb, "- %s [%s]: %s\n", name, p.Severity, p.Instruction)
	}
	return sb.String()
}

// findingToResult converts a BAML finding to a SARIF result
func (w *AnalyzerWrapper) findingToResult(f analyzer.Finding, defaultPath string) sarif.Result {
	path := f.FilePath
	if path == "" {
		path = defaultPath
	}

	return sarif.Result{
		RuleID:  f.RuleID,
		Level:   f.Level,
		Message: sarif.Message{Text: f.Message},
		Locations: []sarif.Location{{
			PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: path},
				Region:           sarif.Region{StartLine: f.StartLine, EndLine: f.EndLine},
			},
		}},
		Properties: map[string]interface{}{
			"gavel/recommendation": f.Recommendation,
			"gavel/explanation":    f.Explanation,
			"gavel/confidence":     f.Confidence,
		},
	}
}
