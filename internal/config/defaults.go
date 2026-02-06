package config

// SystemDefaults returns built-in default policies and provider config.
func SystemDefaults() *Config {
	return &Config{
		Provider: ProviderConfig{
			Name: "ollama",
			Ollama: OllamaConfig{
				Model:   "gpt-oss:20b",
				BaseURL: "http://localhost:11434/v1",
			},
			OpenRouter: OpenRouterConfig{
				Model: "anthropic/claude-sonnet-4",
			},
		},
		Policies: map[string]Policy{
			"shall-be-merged": {
				Description: "Shall this code be merged?",
				Severity:    "error",
				Instruction: "Shall this code be blocked from merging? Flag code that is risky, sloppy, untested, hard to understand, or unecessarily complex. ",
				Enabled:     true,
			},
			"function-length": {
				Description: "Functions should not exceed a reasonable length",
				Severity:    "note",
				Instruction: "Flag functions longer than 50 lines. Consider whether the function could be decomposed.",
				Enabled:     false,
			},
		},
	}
}
