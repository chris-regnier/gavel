package sarif

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
)

func TestAssemble(t *testing.T) {
	results := []Result{
		{RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue A"}},
		{RuleID: "rule-b", Level: "error", Message: Message{Text: "issue B"}},
	}

	rules := []ReportingDescriptor{
		{ID: "rule-a", ShortDescription: Message{Text: "Rule A"}},
		{ID: "rule-b", ShortDescription: Message{Text: "Rule B"}},
	}

	log := Assemble(results, rules, "diff", "code-reviewer")

	if len(log.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name != "gavel" {
		t.Errorf("expected tool name 'gavel', got %q", run.Tool.Driver.Name)
	}
	if len(run.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(run.Results))
	}
	if len(run.Tool.Driver.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(run.Tool.Driver.Rules))
	}
	if run.Properties["gavel/inputScope"] != "diff" {
		t.Errorf("expected inputScope 'diff', got %v", run.Properties["gavel/inputScope"])
	}
	if run.Properties["gavel/persona"] != "code-reviewer" {
		t.Errorf("expected persona 'code-reviewer', got %v", run.Properties["gavel/persona"])
	}
}

func TestAssemble_Dedup(t *testing.T) {
	results := []Result{
		{
			RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 10, EndLine: 15},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.7},
		},
		{
			RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue duplicate"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 12, EndLine: 18},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.9},
		},
	}

	log := Assemble(results, nil, "files", "architect")
	if len(log.Runs[0].Results) != 1 {
		t.Errorf("expected dedup to 1 result, got %d", len(log.Runs[0].Results))
	}
	if log.Runs[0].Results[0].Properties["gavel/confidence"] != 0.9 {
		t.Errorf("expected to keep higher confidence finding")
	}
}

func TestAssembler_AddsCacheMetadata(t *testing.T) {
	results := []Result{
		{
			RuleID:  "test-rule-1",
			Level:   "warning",
			Message: Message{Text: "Test finding"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "test.go"},
				Region:           Region{StartLine: 10, EndLine: 12},
			}}},
			Properties: map[string]interface{}{
				"gavel/confidence":     0.9,
				"gavel/explanation":    "Test explanation",
				"gavel/recommendation": "Test recommendation",
			},
		},
	}

	rules := []ReportingDescriptor{
		{ID: "test-rule-1", ShortDescription: Message{Text: "Test Rule"}},
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: "openrouter",
			OpenRouter: config.OpenRouterConfig{
				Model: "anthropic/claude-sonnet-4",
			},
		},
		Policies: map[string]config.Policy{
			"test-policy": {
				Enabled:     true,
				Instruction: "Test instruction",
			},
		},
	}

	fileHash := "abc123def456" // Mock file content hash
	bamlVersion := "v1.0.0"    // Mock BAML version

	log := NewAssembler().
		WithCacheMetadata(fileHash, cfg, bamlVersion).
		AddResults(results).
		AddRules(rules).
		Build()

	result := log.Runs[0].Results[0]

	// Check cache_key property
	cacheKey, ok := result.Properties["gavel/cache_key"].(string)
	if !ok || cacheKey == "" {
		t.Fatal("Expected gavel/cache_key property")
	}

	// Check analyzer metadata
	analyzer, ok := result.Properties["gavel/analyzer"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected gavel/analyzer property")
	}

	if analyzer["provider"] != "openrouter" {
		t.Errorf("Expected provider=openrouter, got %v", analyzer["provider"])
	}

	if analyzer["model"] != "anthropic/claude-sonnet-4" {
		t.Errorf("Expected model=anthropic/claude-sonnet-4, got %v", analyzer["model"])
	}

	policies, ok := analyzer["policies"].(map[string]interface{})
	if !ok || len(policies) == 0 {
		t.Fatal("Expected policies in analyzer metadata")
	}
}
