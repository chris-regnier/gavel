package config

import (
	"os"
	"strings"
	"testing"
)

func TestMergePolicies_HigherTierOverrides(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Description: "Handle errors",
				Severity:    "warning",
				Instruction: "Check error handling",
				Enabled:     true,
			},
		},
	}
	project := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Severity: "error",
			},
		},
	}
	merged := MergeConfigs(system, project)
	pol := merged.Policies["error-handling"]
	if pol.Severity != "error" {
		t.Errorf("expected severity 'error', got %q", pol.Severity)
	}
	if pol.Description != "Handle errors" {
		t.Errorf("expected description preserved, got %q", pol.Description)
	}
	if !pol.Enabled {
		t.Error("expected enabled to remain true")
	}
}

func TestMergePolicies_HigherTierAddsNew(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {Description: "Handle errors", Severity: "warning", Instruction: "Check error handling", Enabled: true},
		},
	}
	project := &Config{
		Policies: map[string]Policy{
			"function-length": {Description: "Keep functions short", Severity: "note", Instruction: "Flag long functions", Enabled: true},
		},
	}
	merged := MergeConfigs(system, project)
	if len(merged.Policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(merged.Policies))
	}
}

func TestMergePolicies_DisablePolicy(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {Description: "Handle errors", Severity: "warning", Instruction: "Check error handling", Enabled: true},
		},
	}
	project := &Config{
		Policies: map[string]Policy{
			"error-handling": {Enabled: false},
		},
	}
	merged := MergeConfigs(system, project)
	if merged.Policies["error-handling"].Enabled {
		t.Error("expected policy to be disabled")
	}
}

func TestLoadFromFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/policies.yaml"
	os.WriteFile(path, []byte("policies:\n  test-policy:\n    description: \"Test\"\n    severity: \"warning\"\n    instruction: \"Do the thing\"\n    enabled: true\n"), 0644)
	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(cfg.Policies))
	}
}

func TestLoadFromFile_Missing(t *testing.T) {
	cfg, err := LoadFromFile("/nonexistent/path.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Error("expected nil config for missing file")
	}
}

func TestLoadTiered(t *testing.T) {
	dir := t.TempDir()
	machineConf := dir + "/machine.yaml"
	os.WriteFile(machineConf, []byte("policies:\n  error-handling:\n    severity: \"error\"\n"), 0644)
	projectConf := dir + "/project.yaml"
	os.WriteFile(projectConf, []byte("policies:\n  custom-rule:\n    description: \"Custom\"\n    severity: \"warning\"\n    instruction: \"Check custom thing\"\n    enabled: true\n"), 0644)

	cfg, err := LoadTiered(machineConf, projectConf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Policies["error-handling"].Severity != "error" {
		t.Errorf("expected machine override severity 'error', got %q", cfg.Policies["error-handling"].Severity)
	}
	if _, ok := cfg.Policies["custom-rule"]; !ok {
		t.Error("expected project policy 'custom-rule'")
	}
	if _, ok := cfg.Policies["function-length"]; !ok {
		t.Error("expected system default 'function-length'")
	}
}

func TestLoadFromFile_WithProvider(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/policies.yaml"
	yaml := `provider:
  name: ollama
  ollama:
    model: test-model
    base_url: http://test:1234
  openrouter:
    model: test-router-model
policies:
  test-policy:
    description: "Test"
    severity: "warning"
    instruction: "Do the thing"
    enabled: true
`
	os.WriteFile(path, []byte(yaml), 0644)
	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider.Name != "ollama" {
		t.Errorf("expected provider name 'ollama', got %q", cfg.Provider.Name)
	}
	if cfg.Provider.Ollama.Model != "test-model" {
		t.Errorf("expected ollama model 'test-model', got %q", cfg.Provider.Ollama.Model)
	}
	if cfg.Provider.Ollama.BaseURL != "http://test:1234" {
		t.Errorf("expected base_url 'http://test:1234', got %q", cfg.Provider.Ollama.BaseURL)
	}
	if cfg.Provider.OpenRouter.Model != "test-router-model" {
		t.Errorf("expected openrouter model 'test-router-model', got %q", cfg.Provider.OpenRouter.Model)
	}
}

func TestConfig_Validate_ValidOllama(t *testing.T) {
	cfg := &Config{
		Provider: ProviderConfig{
			Name: "ollama",
			Ollama: OllamaConfig{
				Model:   "test-model",
				BaseURL: "http://localhost:11434",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestConfig_Validate_ValidOpenRouter(t *testing.T) {
	os.Setenv("OPENROUTER_API_KEY", "test-key")
	defer os.Unsetenv("OPENROUTER_API_KEY")

	cfg := &Config{
		Provider: ProviderConfig{
			Name: "openrouter",
			OpenRouter: OpenRouterConfig{
				Model: "anthropic/claude-sonnet-4",
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestConfig_Validate_InvalidProviderName(t *testing.T) {
	cfg := &Config{
		Provider: ProviderConfig{
			Name: "invalid",
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for invalid provider name")
	}
	if !strings.Contains(err.Error(), "must be 'ollama' or 'openrouter'") {
		t.Errorf("expected specific error message, got: %v", err)
	}
}

func TestConfig_Validate_OllamaMissingModel(t *testing.T) {
	cfg := &Config{
		Provider: ProviderConfig{
			Name: "ollama",
			Ollama: OllamaConfig{
				BaseURL: "http://localhost:11434",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for missing ollama model")
	}
	if !strings.Contains(err.Error(), "provider.ollama.model is required") {
		t.Errorf("expected specific error message, got: %v", err)
	}
}

func TestConfig_Validate_OpenRouterMissingAPIKey(t *testing.T) {
	os.Unsetenv("OPENROUTER_API_KEY")

	cfg := &Config{
		Provider: ProviderConfig{
			Name: "openrouter",
			OpenRouter: OpenRouterConfig{
				Model: "anthropic/claude-sonnet-4",
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for missing OPENROUTER_API_KEY")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Errorf("expected specific error message, got: %v", err)
	}
}
