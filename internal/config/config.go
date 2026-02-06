package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Policy defines a single analysis policy.
// ContextSelector defines a glob pattern for additional context files
// and optional filters for which artifacts should receive this context.
type ContextSelector struct {
	// Pattern is a glob pattern for files to include as context (e.g., "docs/*", "*.md")
	Pattern string `yaml:"pattern"`

	// OnlyFor is an optional glob pattern - if set, only artifacts matching this pattern
	// will receive the additional context (e.g., "*.go", "*.md")
	OnlyFor string `yaml:"only_for,omitempty"`
}

type Policy struct {
	Description        string            `yaml:"description"`
	Severity           string            `yaml:"severity"`
	Instruction        string            `yaml:"instruction"`
	Enabled            bool              `yaml:"enabled"`
	AdditionalContexts []ContextSelector `yaml:"additional_contexts,omitempty"`
}

// Config holds the full gavel configuration.
type Config struct {
	Provider ProviderConfig    `yaml:"provider"`
	Persona  string            `yaml:"persona"` // AI expert role
	Policies map[string]Policy `yaml:"policies"`
}

// ProviderConfig specifies which LLM provider to use
type ProviderConfig struct {
	Name       string            `yaml:"name"`
	Ollama     OllamaConfig      `yaml:"ollama"`
	OpenRouter OpenRouterConfig  `yaml:"openrouter"`
	Anthropic  AnthropicConfig   `yaml:"anthropic"`
	Bedrock    BedrockConfig     `yaml:"bedrock"`
	OpenAI     OpenAIConfig      `yaml:"openai"`
}

// OllamaConfig holds Ollama-specific settings
type OllamaConfig struct {
	Model   string `yaml:"model"`
	BaseURL string `yaml:"base_url"`
}

// OpenRouterConfig holds OpenRouter-specific settings
type OpenRouterConfig struct {
	Model string `yaml:"model"`
}

// AnthropicConfig holds Anthropic API-specific settings
type AnthropicConfig struct {
	Model string `yaml:"model"`
}

// BedrockConfig holds AWS Bedrock-specific settings
type BedrockConfig struct {
	Model  string `yaml:"model"`
	Region string `yaml:"region"`
}

// OpenAIConfig holds OpenAI API-specific settings
type OpenAIConfig struct {
	Model string `yaml:"model"`
}

// Validate checks that the configuration is valid and ready to use
func (c *Config) Validate() error {
	validProviders := map[string]bool{
		"ollama":     true,
		"openrouter": true,
		"anthropic":  true,
		"bedrock":    true,
		"openai":     true,
	}

	if !validProviders[c.Provider.Name] {
		return fmt.Errorf("provider.name must be one of: ollama, openrouter, anthropic, bedrock, openai; got: %s", c.Provider.Name)
	}

	switch c.Provider.Name {
	case "ollama":
		if c.Provider.Ollama.Model == "" {
			return fmt.Errorf("provider.ollama.model is required when using Ollama")
		}
	case "openrouter":
		if os.Getenv("OPENROUTER_API_KEY") == "" {
			return fmt.Errorf("OPENROUTER_API_KEY environment variable required for OpenRouter")
		}
	case "anthropic":
		if c.Provider.Anthropic.Model == "" {
			return fmt.Errorf("provider.anthropic.model is required when using Anthropic")
		}
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			return fmt.Errorf("ANTHROPIC_API_KEY environment variable required for Anthropic")
		}
	case "bedrock":
		if c.Provider.Bedrock.Model == "" {
			return fmt.Errorf("provider.bedrock.model is required when using Bedrock")
		}
		if c.Provider.Bedrock.Region == "" {
			return fmt.Errorf("provider.bedrock.region is required when using Bedrock")
		}
		// AWS credentials are typically loaded from environment or ~/.aws/credentials
		// We'll validate them at runtime when making the actual call
	case "openai":
		if c.Provider.OpenAI.Model == "" {
			return fmt.Errorf("provider.openai.model is required when using OpenAI")
		}
		if os.Getenv("OPENAI_API_KEY") == "" {
			return fmt.Errorf("OPENAI_API_KEY environment variable required for OpenAI")
		}
	}

	// Validate persona field
	validPersonas := map[string]bool{
		"code-reviewer": true,
		"architect":     true,
		"security":      true,
	}
	if c.Persona != "" && !validPersonas[c.Persona] {
		return fmt.Errorf("unknown persona: %s (valid: code-reviewer, architect, security)", c.Persona)
	}

	return nil
}

// MergeConfigs merges configs in order of increasing precedence.
// Later configs override earlier ones. Non-zero string fields override;
// Enabled always takes effect from the higher tier.
func MergeConfigs(configs ...*Config) *Config {
	result := &Config{
		Policies: make(map[string]Policy),
	}

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		// Merge provider config - non-empty string fields override
		if cfg.Provider.Name != "" {
			result.Provider.Name = cfg.Provider.Name
		}
		if cfg.Provider.Ollama.Model != "" {
			result.Provider.Ollama.Model = cfg.Provider.Ollama.Model
		}
		if cfg.Provider.Ollama.BaseURL != "" {
			result.Provider.Ollama.BaseURL = cfg.Provider.Ollama.BaseURL
		}
		if cfg.Provider.OpenRouter.Model != "" {
			result.Provider.OpenRouter.Model = cfg.Provider.OpenRouter.Model
		}
		if cfg.Provider.Anthropic.Model != "" {
			result.Provider.Anthropic.Model = cfg.Provider.Anthropic.Model
		}
		if cfg.Provider.Bedrock.Model != "" {
			result.Provider.Bedrock.Model = cfg.Provider.Bedrock.Model
		}
		if cfg.Provider.Bedrock.Region != "" {
			result.Provider.Bedrock.Region = cfg.Provider.Bedrock.Region
		}
		if cfg.Provider.OpenAI.Model != "" {
			result.Provider.OpenAI.Model = cfg.Provider.OpenAI.Model
		}

		// Merge persona - non-empty string overrides
		if cfg.Persona != "" {
			result.Persona = cfg.Persona
		}

		// Merge policies (existing logic)
		for name, policy := range cfg.Policies {
			existing, ok := result.Policies[name]
			if !ok {
				result.Policies[name] = policy
				continue
			}
			// Merge: non-zero string fields from higher tier override
			if policy.Description != "" {
				existing.Description = policy.Description
			}
			if policy.Severity != "" {
				existing.Severity = policy.Severity
			}
			if policy.Instruction != "" {
				existing.Instruction = policy.Instruction
			}
			// Enabled: if the higher tier explicitly sets Enabled to true, use it.
			// If Enabled is false (the zero value), only apply it when no string
			// fields are set—indicating a deliberate disable directive rather than
			// an unset default.
			if policy.Enabled {
				existing.Enabled = true
			} else if policy.Description == "" && policy.Severity == "" && policy.Instruction == "" {
				existing.Enabled = false
			}
			// AdditionalContexts: if specified, override completely
			if len(policy.AdditionalContexts) > 0 {
				existing.AdditionalContexts = policy.AdditionalContexts
			}
			result.Policies[name] = existing
		}
	}

	return result
}

// LoadFromFile reads a YAML config file. Returns nil, nil if the file doesn't exist.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return &cfg, nil
}

// LoadTiered loads system defaults, then machine config, then project config,
// and merges them in order of increasing precedence.
func LoadTiered(machinePath, projectPath string) (*Config, error) {
	system := SystemDefaults()

	machine, err := LoadFromFile(machinePath)
	if err != nil {
		return nil, fmt.Errorf("loading machine config: %w", err)
	}

	project, err := LoadFromFile(projectPath)
	if err != nil {
		return nil, fmt.Errorf("loading project config: %w", err)
	}

	return MergeConfigs(system, machine, project), nil
}
