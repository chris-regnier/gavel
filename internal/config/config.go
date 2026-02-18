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

// TelemetryConfig holds OpenTelemetry configuration.
type TelemetryConfig struct {
	Enabled        bool              `yaml:"enabled"`
	Endpoint       string            `yaml:"endpoint"`
	Protocol       string            `yaml:"protocol"`        // "grpc" or "http"
	Insecure       bool              `yaml:"insecure"`
	ServiceName    string            `yaml:"service_name"`
	ServiceVersion string            `yaml:"service_version"`
	SampleRate     float64           `yaml:"sample_rate"`
	Headers        map[string]string `yaml:"headers"`
}

// Config holds the full gavel configuration.
type Config struct {
	Provider    ProviderConfig      `yaml:"provider"`
	Persona     string              `yaml:"persona"` // AI expert role
	Policies    map[string]Policy   `yaml:"policies"`
	LSP         LSPConfig           `yaml:"lsp"`
	RemoteCache RemoteCacheConfig   `yaml:"remote_cache"`
	Telemetry   TelemetryConfig     `yaml:"telemetry"`
}

// RemoteCacheConfig holds remote cache server settings
type RemoteCacheConfig struct {
	Enabled  bool               `yaml:"enabled"`
	URL      string             `yaml:"url"`
	Auth     RemoteCacheAuth    `yaml:"auth"`
	Strategy CacheStrategy      `yaml:"strategy"`
}

// RemoteCacheAuth holds authentication settings for the remote cache
type RemoteCacheAuth struct {
	Type      string `yaml:"type"`       // "bearer", "api_key", or empty for none
	Token     string `yaml:"token"`      // Direct token value
	TokenFile string `yaml:"token_file"` // Path to file containing token
}

// CacheStrategy controls how local and remote caches interact
type CacheStrategy struct {
	WriteToRemote        bool `yaml:"write_to_remote"`
	ReadFromRemote       bool `yaml:"read_from_remote"`
	PreferLocal          bool `yaml:"prefer_local"`
	WarmLocalOnRemoteHit bool `yaml:"warm_local_on_remote_hit"`
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

// LSPConfig holds LSP-specific configuration
type LSPConfig struct {
	Watcher  WatcherConfig  `yaml:"watcher"`
	Analysis AnalysisConfig `yaml:"analysis"`
	Cache    CacheConfig    `yaml:"cache"`
}

// WatcherConfig holds file watcher settings
type WatcherConfig struct {
	DebounceDuration string   `yaml:"debounce_duration"`
	WatchPatterns    []string `yaml:"watch_patterns"`
	IgnorePatterns   []string `yaml:"ignore_patterns"`
}

// AnalysisConfig holds analysis execution settings
type AnalysisConfig struct {
	ParallelFiles int    `yaml:"parallel_files"`
	Priority      string `yaml:"priority"`
}

// CacheConfig holds cache settings
type CacheConfig struct {
	TTL       string `yaml:"ttl"`
	MaxSizeMB int    `yaml:"max_size_mb"`
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

		// Merge LSP config - non-empty fields override
		if cfg.LSP.Watcher.DebounceDuration != "" {
			result.LSP.Watcher.DebounceDuration = cfg.LSP.Watcher.DebounceDuration
		}
		if len(cfg.LSP.Watcher.WatchPatterns) > 0 {
			result.LSP.Watcher.WatchPatterns = cfg.LSP.Watcher.WatchPatterns
		}
		if len(cfg.LSP.Watcher.IgnorePatterns) > 0 {
			result.LSP.Watcher.IgnorePatterns = cfg.LSP.Watcher.IgnorePatterns
		}
		if cfg.LSP.Analysis.ParallelFiles > 0 {
			result.LSP.Analysis.ParallelFiles = cfg.LSP.Analysis.ParallelFiles
		}
		if cfg.LSP.Analysis.Priority != "" {
			result.LSP.Analysis.Priority = cfg.LSP.Analysis.Priority
		}
		if cfg.LSP.Cache.TTL != "" {
			result.LSP.Cache.TTL = cfg.LSP.Cache.TTL
		}
		if cfg.LSP.Cache.MaxSizeMB > 0 {
			result.LSP.Cache.MaxSizeMB = cfg.LSP.Cache.MaxSizeMB
		}

		// Merge remote cache config
		if cfg.RemoteCache.Enabled {
			result.RemoteCache.Enabled = true
		}
		if cfg.RemoteCache.URL != "" {
			result.RemoteCache.URL = cfg.RemoteCache.URL
		}
		if cfg.RemoteCache.Auth.Type != "" {
			result.RemoteCache.Auth.Type = cfg.RemoteCache.Auth.Type
		}
		if cfg.RemoteCache.Auth.Token != "" {
			result.RemoteCache.Auth.Token = cfg.RemoteCache.Auth.Token
		}
		if cfg.RemoteCache.Auth.TokenFile != "" {
			result.RemoteCache.Auth.TokenFile = cfg.RemoteCache.Auth.TokenFile
		}
		// Strategy booleans - only override if the whole RemoteCache section is present
		if cfg.RemoteCache.URL != "" || cfg.RemoteCache.Enabled {
			result.RemoteCache.Strategy = cfg.RemoteCache.Strategy
		}

		// Merge telemetry config - non-empty string fields override.
		// If the telemetry section is explicitly present (endpoint or service_name set),
		// boolean and float fields are applied as-is to allow disabling.
		telemetrySectionPresent := cfg.Telemetry.Endpoint != "" || cfg.Telemetry.ServiceName != ""
		if telemetrySectionPresent || cfg.Telemetry.Enabled {
			result.Telemetry.Enabled = cfg.Telemetry.Enabled
		}
		if cfg.Telemetry.Endpoint != "" {
			result.Telemetry.Endpoint = cfg.Telemetry.Endpoint
		}
		if cfg.Telemetry.Protocol != "" {
			result.Telemetry.Protocol = cfg.Telemetry.Protocol
		}
		if telemetrySectionPresent {
			result.Telemetry.Insecure = cfg.Telemetry.Insecure
		} else if cfg.Telemetry.Insecure {
			result.Telemetry.Insecure = true
		}
		if cfg.Telemetry.ServiceName != "" {
			result.Telemetry.ServiceName = cfg.Telemetry.ServiceName
		}
		if cfg.Telemetry.ServiceVersion != "" {
			result.Telemetry.ServiceVersion = cfg.Telemetry.ServiceVersion
		}
		if telemetrySectionPresent || cfg.Telemetry.SampleRate != 0 {
			result.Telemetry.SampleRate = cfg.Telemetry.SampleRate
		}
		if len(cfg.Telemetry.Headers) > 0 {
			result.Telemetry.Headers = cfg.Telemetry.Headers
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

// GetRemoteCacheToken returns the authentication token for the remote cache.
// It checks the Token field first, then reads from TokenFile if specified.
func (c *RemoteCacheConfig) GetRemoteCacheToken() (string, error) {
	if c.Auth.Token != "" {
		return c.Auth.Token, nil
	}
	if c.Auth.TokenFile != "" {
		// Expand ~ to home directory
		tokenFile := c.Auth.TokenFile
		if len(tokenFile) > 0 && tokenFile[0] == '~' {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("expanding home directory: %w", err)
			}
			tokenFile = home + tokenFile[1:]
		}

		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("reading token file %s: %w", tokenFile, err)
		}
		// Trim whitespace from token
		return string(data[:len(data)-countTrailingNewlines(data)]), nil
	}
	return "", nil
}

// countTrailingNewlines counts trailing newline characters
func countTrailingNewlines(data []byte) int {
	count := 0
	for i := len(data) - 1; i >= 0 && (data[i] == '\n' || data[i] == '\r'); i-- {
		count++
	}
	return count
}
