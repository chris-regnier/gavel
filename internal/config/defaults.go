package config

// SystemDefaults returns built-in default policies and provider config.
func SystemDefaults() *Config {
	return &Config{
		Provider: ProviderConfig{
			Name: "ollama",
			Ollama: OllamaConfig{
				Model:   "qwen2.5-coder:7b",
				BaseURL: "http://localhost:11434/v1",
			},
			OpenRouter: OpenRouterConfig{
				Model: "anthropic/claude-haiku-4-5",
			},
			Anthropic: AnthropicConfig{
				Model: "claude-haiku-4-5",
			},
			Bedrock: BedrockConfig{
				Model:  "anthropic.claude-haiku-4-5-v1:0",
				Region: "us-east-1",
			},
			OpenAI: OpenAIConfig{
				Model: "gpt-5.2",
			},
			Anthropic: AnthropicConfig{
				Model: "claude-sonnet-4-20250514",
			},
			Bedrock: BedrockConfig{
				Model:  "anthropic.claude-sonnet-4-5-v2:0",
				Region: "us-east-1",
			},
			OpenAI: OpenAIConfig{
				Model: "gpt-4o",
			},
		},
		Persona: "code-reviewer",
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
