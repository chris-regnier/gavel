package config

// SystemDefaults returns built-in default policies.
func SystemDefaults() *Config {
	return &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Description: "Public functions must handle errors explicitly",
				Severity:    "warning",
				Instruction: "Check that all public functions either return an error or handle errors from called functions. Flag functions that silently discard errors.",
				Enabled:     true,
			},
			"function-length": {
				Description: "Functions should not exceed a reasonable length",
				Severity:    "note",
				Instruction: "Flag functions longer than 50 lines. Consider whether the function could be decomposed.",
				Enabled:     true,
			},
		},
	}
}
