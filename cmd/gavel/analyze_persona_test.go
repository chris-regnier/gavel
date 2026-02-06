package main

import (
	"os"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
)

// TestPersonaConfigLoading verifies that persona configuration is properly loaded
// from config files and merged according to tier precedence.
func TestPersonaConfigLoading(t *testing.T) {
	dir := t.TempDir()
	machineConf := dir + "/machine.yaml"
	projectConf := dir + "/project.yaml"

	// Machine config sets persona to architect
	machineYAML := `provider:
  name: ollama
  ollama:
    model: test-model
    base_url: http://localhost:11434
persona: architect
policies:
  shall-be-merged:
    enabled: true
    severity: error
`
	if err := os.WriteFile(machineConf, []byte(machineYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Project config overrides to security
	projectYAML := `persona: security
`
	if err := os.WriteFile(projectConf, []byte(projectYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadTiered(machineConf, projectConf)
	if err != nil {
		t.Fatalf("LoadTiered failed: %v", err)
	}

	// Project config should override machine config
	if cfg.Persona != "security" {
		t.Errorf("expected persona 'security' from project override, got %q", cfg.Persona)
	}
}

// TestPersonaValidation verifies that only valid personas are accepted
// and invalid personas are rejected with appropriate error messages.
func TestPersonaValidation(t *testing.T) {
	tests := []struct {
		name        string
		persona     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid code-reviewer",
			persona:     "code-reviewer",
			expectError: false,
		},
		{
			name:        "valid architect",
			persona:     "architect",
			expectError: false,
		},
		{
			name:        "valid security",
			persona:     "security",
			expectError: false,
		},
		{
			name:        "invalid persona",
			persona:     "hacker",
			expectError: true,
			errorMsg:    "unknown persona",
		},
		{
			name:        "empty persona uses default",
			persona:     "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Provider: config.ProviderConfig{
					Name: "ollama",
					Ollama: config.OllamaConfig{
						Model:   "test-model",
						BaseURL: "http://localhost:11434",
					},
				},
				Persona: tt.persona,
			}

			err := cfg.Validate()
			if tt.expectError {
				if err == nil {
					t.Error("expected validation error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no validation error, got: %v", err)
				}
			}
		})
	}
}

// TestPersonaOverrideFromCLI simulates CLI flag override behavior.
// This tests the pattern used in analyze.go where CLI flags override config.
func TestPersonaOverrideFromCLI(t *testing.T) {
	dir := t.TempDir()
	configPath := dir + "/policies.yaml"

	// Config file sets persona to code-reviewer
	configYAML := `provider:
  name: ollama
  ollama:
    model: test-model
    base_url: http://localhost:11434
persona: code-reviewer
policies:
  shall-be-merged:
    enabled: true
    severity: error
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify initial persona from config
	if cfg.Persona != "code-reviewer" {
		t.Errorf("expected persona 'code-reviewer' from config, got %q", cfg.Persona)
	}

	// Simulate CLI flag override (as done in analyze.go)
	cliPersona := "security"
	if cliPersona != "" {
		cfg.Persona = cliPersona
	}

	// Verify CLI override took effect
	if cfg.Persona != "security" {
		t.Errorf("expected persona 'security' from CLI override, got %q", cfg.Persona)
	}

	// Validate the overridden config
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config after CLI override, got error: %v", err)
	}
}

// TestPersonaMergingBehavior verifies the tier merging behavior for persona field
// as implemented in config.MergeConfigs.
func TestPersonaMergingBehavior(t *testing.T) {
	tests := []struct {
		name           string
		systemPersona  string
		machinePersona string
		projectPersona string
		expected       string
	}{
		{
			name:           "project overrides machine and system",
			systemPersona:  "code-reviewer",
			machinePersona: "architect",
			projectPersona: "security",
			expected:       "security",
		},
		{
			name:           "machine overrides system when no project",
			systemPersona:  "code-reviewer",
			machinePersona: "architect",
			projectPersona: "",
			expected:       "architect",
		},
		{
			name:           "system default when no overrides",
			systemPersona:  "code-reviewer",
			machinePersona: "",
			projectPersona: "",
			expected:       "code-reviewer",
		},
		{
			name:           "empty project preserves machine",
			systemPersona:  "code-reviewer",
			machinePersona: "security",
			projectPersona: "",
			expected:       "security",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			system := &config.Config{Persona: tt.systemPersona}
			machine := &config.Config{Persona: tt.machinePersona}
			project := &config.Config{Persona: tt.projectPersona}

			merged := config.MergeConfigs(system, machine, project)

			if merged.Persona != tt.expected {
				t.Errorf("expected persona %q, got %q", tt.expected, merged.Persona)
			}
		})
	}
}

// TestPersonaWithInvalidProvider verifies that persona validation
// happens after provider validation, and both must be valid.
func TestPersonaWithInvalidProvider(t *testing.T) {
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: "invalid-provider",
		},
		Persona: "code-reviewer",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for invalid provider")
	}
	// Provider validation should fail first
	if !strings.Contains(err.Error(), "provider.name must be") {
		t.Errorf("expected provider validation error, got: %v", err)
	}
}

// TestPersonaInSystemDefaults verifies that system defaults include
// a valid default persona (code-reviewer).
func TestPersonaInSystemDefaults(t *testing.T) {
	cfg := config.SystemDefaults()

	if cfg.Persona != "code-reviewer" {
		t.Errorf("expected default persona 'code-reviewer', got %q", cfg.Persona)
	}

	// Verify the default persona is valid
	if err := cfg.Validate(); err != nil {
		// Skip OpenRouter API key validation for this test
		if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
			t.Errorf("expected valid default config (except for API key), got error: %v", err)
		}
	}
}
