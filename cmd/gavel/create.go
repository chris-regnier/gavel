package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	baml_client "github.com/chris-regnier/gavel/baml_client"
	"github.com/chris-regnier/gavel/internal/config"
)

var (
	flagCreateOutput    string
	flagCreateProvider  string
	flagCreateLanguages string
	flagCreateCategory  string
)

func init() {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create gavel configuration components using AI",
		Long: `Create policies, rules, personas, and configurations using natural language descriptions.

Examples:
  gavel create policy "Check that all public functions have documentation comments"
  gavel create rule --category=security "Detect hardcoded JWT secrets in Go code"
  gavel create persona "A React expert who focuses on hooks and performance"
  gavel create config "I want to analyze Go microservices for security issues"`,
	}

	// Policy subcommand
	policyCmd := &cobra.Command{
		Use:   "policy [description]",
		Short: "Create a new policy from a description",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runCreatePolicy,
	}
	policyCmd.Flags().StringVarP(&flagCreateOutput, "output", "o", "", "Output file (default: stdout)")

	// Rule subcommand
	ruleCmd := &cobra.Command{
		Use:   "rule [description]",
		Short: "Create a new regex-based rule from a description",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runCreateRule,
	}
	ruleCmd.Flags().StringVarP(&flagCreateOutput, "output", "o", "", "Output file (default: stdout)")
	ruleCmd.Flags().StringVarP(&flagCreateCategory, "category", "c", "maintainability", "Rule category (security, reliability, maintainability)")
	ruleCmd.Flags().StringVarP(&flagCreateLanguages, "languages", "l", "any", "Target languages (comma-separated, e.g., 'go,python')")

	// Persona subcommand
	personaCmd := &cobra.Command{
		Use:   "persona [description]",
		Short: "Create a new persona from a description",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runCreatePersona,
	}
	personaCmd.Flags().StringVarP(&flagCreateOutput, "output", "o", "", "Output file (default: stdout)")

	// Config subcommand
	configCmd := &cobra.Command{
		Use:   "config [requirements]",
		Short: "Create a complete configuration from requirements",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runCreateConfig,
	}
	configCmd.Flags().StringVarP(&flagCreateOutput, "output", "o", ".gavel/policies.yaml", "Output file")
	configCmd.Flags().StringVarP(&flagCreateProvider, "provider", "p", "", "Preferred provider (ollama, openrouter, anthropic, bedrock, openai)")

	// Wizard subcommand (TUI)
	wizardCmd := &cobra.Command{
		Use:   "wizard",
		Short: "Launch interactive TUI wizard for creating configurations",
		RunE:  runCreateWizard,
	}

	createCmd.AddCommand(policyCmd, ruleCmd, personaCmd, configCmd, wizardCmd)
	rootCmd.AddCommand(createCmd)
}

func runCreatePolicy(cmd *cobra.Command, args []string) error {
	description := strings.Join(args, " ")
	ctx := context.Background()

	// Check for API key
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable required for AI generation")
	}

	fmt.Fprintln(os.Stderr, "Generating policy...")

	policy, err := baml_client.GeneratePolicy(ctx, description)
	if err != nil {
		return fmt.Errorf("generating policy: %w", err)
	}

	// Convert to config.Policy format
	cfgPolicy := config.Policy{
		Description: policy.Description,
		Severity:    policy.Severity,
		Instruction: policy.Instruction,
		Enabled:     policy.Enabled,
	}

	// Output as YAML
	yamlData, err := yaml.Marshal(map[string]config.Policy{policy.Id: cfgPolicy})
	if err != nil {
		return fmt.Errorf("marshaling policy: %w", err)
	}

	return writeOutput(string(yamlData), flagCreateOutput)
}

func runCreateRule(cmd *cobra.Command, args []string) error {
	description := strings.Join(args, " ")
	ctx := context.Background()

	// Check for API key
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable required for AI generation")
	}

	languages := flagCreateLanguages
	if languages == "any" {
		languages = ""
	}

	fmt.Fprintln(os.Stderr, "Generating rule...")

	rule, err := baml_client.GenerateRule(ctx, description, flagCreateCategory, languages)
	if err != nil {
		return fmt.Errorf("generating rule: %w", err)
	}

	// Convert to map for YAML output
	ruleMap := map[string]interface{}{
		"rules": []map[string]interface{}{{
			"id":          rule.Id,
			"name":        rule.Name,
			"category":    rule.Category,
			"pattern":     rule.Pattern,
			"level":       rule.Level,
			"confidence":  rule.Confidence,
			"message":     rule.Message,
			"explanation": rule.Explanation,
			"remediation": rule.Remediation,
			"source":      rule.Source,
		}},
	}

	if len(rule.Languages) > 0 {
		ruleMap["rules"].([]map[string]interface{})[0]["languages"] = rule.Languages
	}
	if len(rule.Cwe) > 0 {
		ruleMap["rules"].([]map[string]interface{})[0]["cwe"] = rule.Cwe
	}
	if len(rule.Owasp) > 0 {
		ruleMap["rules"].([]map[string]interface{})[0]["owasp"] = rule.Owasp
	}
	if len(rule.References) > 0 {
		ruleMap["rules"].([]map[string]interface{})[0]["references"] = rule.References
	}

	yamlData, err := yaml.Marshal(ruleMap)
	if err != nil {
		return fmt.Errorf("marshaling rule: %w", err)
	}

	return writeOutput(string(yamlData), flagCreateOutput)
}

func runCreatePersona(cmd *cobra.Command, args []string) error {
	description := strings.Join(args, " ")
	ctx := context.Background()

	// Check for API key
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable required for AI generation")
	}

	// Extract focus areas from description (simple heuristic)
	focusAreas := extractFocusAreas(description)

	fmt.Fprintln(os.Stderr, "Generating persona...")

	persona, err := baml_client.GeneratePersona(ctx, description, focusAreas)
	if err != nil {
		return fmt.Errorf("generating persona: %w", err)
	}

	// Output as structured data
	output := map[string]interface{}{
		"name":           persona.Name,
		"display_name":   persona.Display_name,
		"system_prompt":  persona.System_prompt,
	}

	yamlData, err := yaml.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling persona: %w", err)
	}

	return writeOutput(string(yamlData), flagCreateOutput)
}

func runCreateConfig(cmd *cobra.Command, args []string) error {
	requirements := strings.Join(args, " ")
	ctx := context.Background()

	// Check for API key
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable required for AI generation")
	}

	fmt.Fprintln(os.Stderr, "Generating configuration...")

	genConfig, err := baml_client.GenerateConfig(ctx, requirements, flagCreateProvider)
	if err != nil {
		return fmt.Errorf("generating config: %w", err)
	}

	// Convert to config.Config format
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: genConfig.Provider.Provider_name,
		},
		Persona:  genConfig.Persona,
		Policies: make(map[string]config.Policy),
	}

	// Set provider-specific config
	switch genConfig.Provider.Provider_name {
	case "ollama":
		cfg.Provider.Ollama = config.OllamaConfig{
			Model:   genConfig.Provider.Model,
			BaseURL: genConfig.Provider.Base_url,
		}
		if cfg.Provider.Ollama.BaseURL == "" {
			cfg.Provider.Ollama.BaseURL = "http://localhost:11434/v1"
		}
	case "openrouter":
		cfg.Provider.OpenRouter = config.OpenRouterConfig{
			Model: genConfig.Provider.Model,
		}
	case "anthropic":
		cfg.Provider.Anthropic = config.AnthropicConfig{
			Model: genConfig.Provider.Model,
		}
	case "bedrock":
		cfg.Provider.Bedrock = config.BedrockConfig{
			Model:  genConfig.Provider.Model,
			Region: genConfig.Provider.Region,
		}
		if cfg.Provider.Bedrock.Region == "" {
			cfg.Provider.Bedrock.Region = "us-east-1"
		}
	case "openai":
		cfg.Provider.OpenAI = config.OpenAIConfig{
			Model: genConfig.Provider.Model,
		}
	}

	// Add policies
	for _, p := range genConfig.Policies {
		cfg.Policies[p.Id] = config.Policy{
			Description: p.Description,
			Severity:    p.Severity,
			Instruction: p.Instruction,
			Enabled:     p.Enabled,
		}
	}

	// Output as YAML
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Ensure output directory exists
	if flagCreateOutput != "" && flagCreateOutput != "-" {
		dir := filepath.Dir(flagCreateOutput)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	return writeOutput(string(yamlData), flagCreateOutput)
}

func runCreateWizard(cmd *cobra.Command, args []string) error {
	// Check for API key
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		return fmt.Errorf("OPENROUTER_API_KEY environment variable required for AI generation")
	}

	// Launch TUI wizard
	return launchCreateWizard()
}

// writeOutput writes content to file or stdout
func writeOutput(content, outputPath string) error {
	if outputPath == "" || outputPath == "-" {
		fmt.Println(content)
		return nil
	}

	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created: %s\n", outputPath)
	return nil
}

// extractFocusAreas extracts focus areas from description using simple heuristics
func extractFocusAreas(description string) []string {
	// Common technical terms that might indicate focus areas
	keywords := map[string][]string{
		"react":       {"React components", "Hooks patterns", "State management"},
		"performance": {"Performance optimization", "Memory usage", "CPU efficiency"},
		"security":    {"Security vulnerabilities", "Authentication", "Authorization"},
		"api":         {"API design", "REST conventions", "GraphQL patterns"},
		"database":    {"Database queries", "ORM usage", "Transaction handling"},
		"testing":     {"Test coverage", "Unit tests", "Integration tests"},
		"go":          {"Go idioms", "Error handling", "Concurrency"},
		"python":      {"Python idioms", "Type hints", "Async patterns"},
	}

	descLower := strings.ToLower(description)
	var areas []string

	for keyword, focus := range keywords {
		if strings.Contains(descLower, keyword) {
			areas = append(areas, focus...)
		}
	}

	if len(areas) == 0 {
		areas = []string{"Code quality", "Best practices"}
	}

	return areas
}

// JSON helpers for complex types
func toJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
