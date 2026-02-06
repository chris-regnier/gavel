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
	if !strings.Contains(err.Error(), "must be one of") {
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

func TestSystemDefaults_IncludesProvider(t *testing.T) {
	cfg := SystemDefaults()
	if cfg.Provider.Name != "ollama" {
		t.Errorf("expected default provider 'ollama', got %q", cfg.Provider.Name)
	}
	if cfg.Provider.Ollama.Model != "qwen2.5-coder:7b" {
		t.Errorf("expected ollama model 'qwen2.5-coder:7b', got %q", cfg.Provider.Ollama.Model)
	}
	if cfg.Provider.Ollama.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("expected ollama base_url 'http://localhost:11434/v1', got %q", cfg.Provider.Ollama.BaseURL)
	}
	if cfg.Provider.OpenRouter.Model != "anthropic/claude-3-5-haiku-20241022" {
		t.Errorf("expected openrouter model 'anthropic/claude-3-5-haiku-20241022', got %q", cfg.Provider.OpenRouter.Model)
	}
}

func TestMergeConfigs_ProviderOverride(t *testing.T) {
	system := &Config{
		Provider: ProviderConfig{
			Name: "openrouter",
			Ollama: OllamaConfig{
				Model:   "default-ollama",
				BaseURL: "http://localhost:11434",
			},
			OpenRouter: OpenRouterConfig{
				Model: "default-openrouter",
			},
		},
	}
	project := &Config{
		Provider: ProviderConfig{
			Name: "ollama",
			Ollama: OllamaConfig{
				Model: "custom-model",
			},
		},
	}
	merged := MergeConfigs(system, project)

	if merged.Provider.Name != "ollama" {
		t.Errorf("expected provider name 'ollama', got %q", merged.Provider.Name)
	}
	if merged.Provider.Ollama.Model != "custom-model" {
		t.Errorf("expected ollama model 'custom-model', got %q", merged.Provider.Ollama.Model)
	}
	if merged.Provider.Ollama.BaseURL != "http://localhost:11434" {
		t.Errorf("expected base_url preserved from system, got %q", merged.Provider.Ollama.BaseURL)
	}
	if merged.Provider.OpenRouter.Model != "default-openrouter" {
		t.Errorf("expected openrouter model preserved, got %q", merged.Provider.OpenRouter.Model)
	}
}

func TestMergeConfigs_ProviderPartialOverride(t *testing.T) {
	system := &Config{
		Provider: ProviderConfig{
			Name: "openrouter",
			Ollama: OllamaConfig{
				Model:   "default-model",
				BaseURL: "http://localhost:11434",
			},
		},
	}
	machine := &Config{
		Provider: ProviderConfig{
			Ollama: OllamaConfig{
				BaseURL: "http://custom:9999",
			},
		},
	}
	merged := MergeConfigs(system, machine)

	if merged.Provider.Name != "openrouter" {
		t.Errorf("expected provider name preserved, got %q", merged.Provider.Name)
	}
	if merged.Provider.Ollama.BaseURL != "http://custom:9999" {
		t.Errorf("expected base_url overridden, got %q", merged.Provider.Ollama.BaseURL)
	}
	if merged.Provider.Ollama.Model != "default-model" {
		t.Errorf("expected model preserved, got %q", merged.Provider.Ollama.Model)
	}
}

func TestConfigValidation_Persona(t *testing.T) {
	tests := []struct {
		name    string
		persona string
		wantErr bool
	}{
		{"valid code-reviewer", "code-reviewer", false},
		{"valid architect", "architect", false},
		{"valid security", "security", false},
		{"invalid persona", "invalid", true},
		{"empty uses default", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Provider: ProviderConfig{
					Name: "ollama",
					Ollama: OllamaConfig{
						Model:   "test-model",
						BaseURL: "http://localhost:11434",
					},
				},
				Persona: tt.persona,
			}
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), "unknown persona") {
				t.Errorf("expected 'unknown persona' error, got: %v", err)
			}
		})
	}
}

func TestSystemDefaults_IncludesPersona(t *testing.T) {
	cfg := SystemDefaults()
	if cfg.Persona != "code-reviewer" {
		t.Errorf("expected default persona 'code-reviewer', got %q", cfg.Persona)
	}
}

func TestMergeConfigs_PersonaOverride(t *testing.T) {
	system := &Config{
		Persona: "code-reviewer",
	}
	project := &Config{
		Persona: "security",
	}
	merged := MergeConfigs(system, project)

	if merged.Persona != "security" {
		t.Errorf("expected persona 'security', got %q", merged.Persona)
	}
}

func TestMergeConfigs_PersonaPreserved(t *testing.T) {
	system := &Config{
		Persona: "architect",
	}
	project := &Config{
		Persona: "",
	}
	merged := MergeConfigs(system, project)

	if merged.Persona != "architect" {
		t.Errorf("expected persona 'architect' preserved, got %q", merged.Persona)
	}
}
