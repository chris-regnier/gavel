package harness

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/chris-regnier/gavel/internal/config"
)

// VariantConfig represents a single variant configuration for comparison.
// Each variant can differ in persona, policies, provider settings, etc.
type VariantConfig struct {
	// Name is a unique identifier for this variant (e.g., "baseline", "minimal", "filter_on")
	Name string `yaml:"name"`

	// Description is an optional human-readable description
	Description string `yaml:"description,omitempty"`

	// Persona overrides the default persona (e.g., "code-reviewer", "architect", "security")
	Persona string `yaml:"persona,omitempty"`

	// StrictFilter enables/disables the applicability filter
	StrictFilter *bool `yaml:"strict_filter,omitempty"`

	// PromptAdd is appended to the persona prompt (for prompt experiments)
	PromptAdd string `yaml:"prompt_add,omitempty"`

	// PromptReplace completely replaces the persona prompt (for persona experiments)
	PromptReplace string `yaml:"prompt_replace,omitempty"`

	// Policies overrides specific policies (merged with baseline)
	Policies map[string]config.Policy `yaml:"policies,omitempty"`

	// Provider overrides the provider configuration
	Provider *ProviderOverride `yaml:"provider,omitempty"`
}

// ProviderOverride allows overriding specific provider settings for a variant
type ProviderOverride struct {
	Name       string `yaml:"name,omitempty"`
	Ollama     *struct {
		Model   string `yaml:"model,omitempty"`
		BaseURL string `yaml:"base_url,omitempty"`
	} `yaml:"ollama,omitempty"`
	OpenRouter *struct {
		Model string `yaml:"model,omitempty"`
	} `yaml:"openrouter,omitempty"`
	Anthropic *struct {
		Model string `yaml:"model,omitempty"`
	} `yaml:"anthropic,omitempty"`
	Bedrock *struct {
		Model  string `yaml:"model,omitempty"`
		Region string `yaml:"region,omitempty"`
	} `yaml:"bedrock,omitempty"`
	OpenAI *struct {
		Model string `yaml:"model,omitempty"`
	} `yaml:"openai,omitempty"`
}

// RepositoryConfig defines an external repository to clone for analysis
type RepositoryConfig struct {
	// Name is a unique identifier for this repo (used in targets)
	Name string `yaml:"name"`

	// URL is the git repository URL (e.g., https://github.com/juice-shop/juice-shop)
	URL string `yaml:"url"`

	// Branch is the git branch to checkout (default: default branch)
	Branch string `yaml:"branch,omitempty"`

	// Commit is a specific commit hash to checkout (for reproducibility)
	Commit string `yaml:"commit,omitempty"`

	// Tag is a specific tag to checkout
	Tag string `yaml:"tag,omitempty"`

	// Depth is the git clone depth (default: 1 for shallow clone)
	Depth int `yaml:"depth,omitempty"`
}

// TargetConfig defines what to analyze - can be local or from an external repo
type TargetConfig struct {
	// Path is a local directory path (for local analysis)
	Path string `yaml:"path,omitempty"`

	// Repo references an external repository by name
	Repo string `yaml:"repo,omitempty"`

	// Paths are subdirectories within the repo to analyze (default: root)
	Paths []string `yaml:"paths,omitempty"`
}

// HarnessConfig is the top-level configuration for a harness run
type HarnessConfig struct {
	// Variants is the list of variants to compare
	Variants []VariantConfig `yaml:"variants"`

	// Runs is the number of iterations per variant (default: 3)
	Runs int `yaml:"runs"`

	// Packages is the list of packages/directories to analyze (deprecated, use Targets)
	Packages []string `yaml:"packages,omitempty"`

	// Targets is the list of analysis targets (supports local and external repos)
	Targets []TargetConfig `yaml:"targets,omitempty"`

	// Repos is the list of external repositories to clone
	Repos []RepositoryConfig `yaml:"repos,omitempty"`

	// CacheDir is where external repos are cached (default: .gavel/cache)
	CacheDir string `yaml:"cache_dir,omitempty"`

	// OutputDir is where results are written (default: .gavel/results)
	OutputDir string `yaml:"output_dir,omitempty"`

	// Baseline is the name of the baseline variant for comparisons (optional)
	Baseline string `yaml:"baseline,omitempty"`
}

// LoadHarnessConfig loads a harness configuration from YAML
func LoadHarnessConfig(data []byte) (*HarnessConfig, error) {
	var cfg HarnessConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Apply defaults
	if cfg.Runs == 0 {
		cfg.Runs = 3
	}

	return &cfg, nil
}

// LoadHarnessConfigFile loads a harness configuration from a file
func LoadHarnessConfigFile(path string) (*HarnessConfig, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return LoadHarnessConfig(data)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// MergeWithConfig merges a variant configuration with a base config
func (v *VariantConfig) MergeWithConfig(base *config.Config) *config.Config {
	result := config.MergeConfigs(base, &config.Config{})

	if v.Persona != "" {
		result.Persona = v.Persona
	}

	if v.StrictFilter != nil {
		result.StrictFilter = *v.StrictFilter
	}

	// Merge policies
	for name, policy := range v.Policies {
		result.Policies[name] = policy
	}

	// Merge provider overrides
	if v.Provider != nil {
		if v.Provider.Name != "" {
			result.Provider.Name = v.Provider.Name
		}
		if v.Provider.Ollama != nil {
			if v.Provider.Ollama.Model != "" {
				result.Provider.Ollama.Model = v.Provider.Ollama.Model
			}
			if v.Provider.Ollama.BaseURL != "" {
				result.Provider.Ollama.BaseURL = v.Provider.Ollama.BaseURL
			}
		}
		if v.Provider.OpenRouter != nil && v.Provider.OpenRouter.Model != "" {
			result.Provider.OpenRouter.Model = v.Provider.OpenRouter.Model
		}
		if v.Provider.Anthropic != nil && v.Provider.Anthropic.Model != "" {
			result.Provider.Anthropic.Model = v.Provider.Anthropic.Model
		}
		if v.Provider.Bedrock != nil {
			if v.Provider.Bedrock.Model != "" {
				result.Provider.Bedrock.Model = v.Provider.Bedrock.Model
			}
			if v.Provider.Bedrock.Region != "" {
				result.Provider.Bedrock.Region = v.Provider.Bedrock.Region
			}
		}
		if v.Provider.OpenAI != nil && v.Provider.OpenAI.Model != "" {
			result.Provider.OpenAI.Model = v.Provider.OpenAI.Model
		}
	}

	return result
}
