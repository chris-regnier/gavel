// BAML functions for generating gavel configuration components
// These functions take natural language descriptions and generate structured config files

class GeneratedPolicy {
  id string @description("Unique identifier for the policy (snake-case)")
  description string @description("Short description of what this policy checks")
  severity string @description("One of: error, warning, note, none")
  instruction string @description("Detailed instructions for the AI analyzer")
  enabled bool @description("Whether this policy is enabled by default")
}

class GeneratedRule {
  id string @description("Unique rule ID (e.g., CUSTOM-S001)")
  name string @description("Short snake-case name for the rule")
  category string @description("One of: security, reliability, maintainability")
  pattern string @description("Regex pattern to match (Go regex syntax)")
  languages string[] @description("Target languages (e.g., ['go', 'python']), omit for all")
  level string @description("One of: error, warning, note")
  confidence float @description("Confidence score 0.0-1.0 for this pattern")
  message string @description("Short message shown when pattern matches")
  explanation string @description("Detailed explanation of why this is an issue")
  remediation string @description("How to fix or address the issue")
  source string @description("One of: CWE, OWASP, SonarQube, Custom")
  cwe string[] @description("Related CWE IDs (e.g., ['CWE-79', 'CWE-89'])")
  owasp string[] @description("Related OWASP categories (e.g., ['A03:2021'])")
  references string[] @description("URLs to documentation")
}

class GeneratedPersona {
  name string @description("Persona identifier (snake-case, e.g., 'performance-expert')")
  display_name string @description("Human-readable name")
  system_prompt string @description("Complete system prompt for this persona")
}

class GeneratedProviderConfig {
  provider_name string @description("One of: ollama, openrouter, anthropic, bedrock, openai")
  model string @description("Model identifier for the selected provider")
  base_url string @description("Optional base URL (mainly for Ollama)")
  region string @description("Optional region (mainly for Bedrock)")
}

class GeneratedConfig {
  provider GeneratedProviderConfig @description("Provider configuration")
  persona string @description("Default persona to use")
  policies GeneratedPolicy[] @description("Initial policies to include")
}

// Generate a policy from a natural language description
function GeneratePolicy(
  description: string
) -> GeneratedPolicy {
  client OpenRouter
  prompt #"
    You are an expert at creating code analysis policies for AI-powered code review tools.

    Create a policy based on the user's description. The policy should:
    1. Have a clear, specific focus
    2. Include detailed instructions an AI can follow
    3. Use appropriate severity (error for blockers, warning for issues, note for suggestions)
    4. Be enabled by default for important checks

    User's description: {{ description }}

    Generate a complete policy with:
    - id: snake-case identifier (e.g., "check-error-handling", "no-magic-numbers")
    - description: 1-sentence summary
    - severity: error | warning | note | none
    - instruction: Detailed instructions for the AI analyzer (2-4 sentences)
    - enabled: true if this is an important check, false otherwise

    {{ ctx.output_format }}
  "#
}

// Generate a regex-based rule from a natural language description
function GenerateRule(
  description: string,
  category: string,
  languages: string
) -> GeneratedRule {
  client OpenRouter
  prompt #"
    You are an expert at creating static analysis rules using regular expressions.

    Create a regex-based rule based on the user's description. The rule should:
    1. Use Go regex syntax (RE2 compatible)
    2. Be precise enough to catch real issues but avoid excessive false positives
    3. Include clear guidance for developers who encounter it

    User's description: {{ description }}
    Category: {{ category }}
    Target languages: {{ languages }}

    Generate a complete rule with:
    - id: Follow conventions like CUSTOM-S001 (security), CUSTOM-R001 (reliability), CUSTOM-M001 (maintainability)
    - name: Short snake-case identifier
    - category: {{ category }}
    - pattern: A Go regex pattern that matches the issue (be careful with escaping)
    - languages: Array of language identifiers (omit if applicable to all)
    - level: error | warning | note
    - confidence: 0.0-1.0 based on pattern precision
    - message: Concise issue description (1 sentence)
    - explanation: Detailed explanation (2-3 sentences)
    - remediation: Actionable fix guidance
    - source: "Custom" (or CWE/OWASP if applicable)
    - cwe: Related CWE IDs if applicable (e.g., ["CWE-89"] for injection)
    - owasp: Related OWASP categories if applicable (e.g., ["A03:2021"])
    - references: URLs to relevant documentation

    {{ ctx.output_format }}
  "#
}

// Generate a custom persona from a natural language description
function GeneratePersona(
  description: string,
  focus_areas: string[]
) -> GeneratedPersona {
  client OpenRouter
  prompt #"
    You are an expert at creating AI system prompts for specialized code analysis roles.

    Create a persona based on the user's description. The persona should:
    1. Define a clear expert role with specific expertise
    2. Include focus areas relevant to that role
    3. Define an appropriate tone for the expert type
    4. Include confidence guidance for different finding severities

    User's description: {{ description }}
    Focus areas: {{ focus_areas }}

    Generate a complete persona with:
    - name: snake-case identifier (e.g., "performance-expert", "api-reviewer")
    - display_name: Human-readable name (e.g., "Performance Expert")
    - system_prompt: Complete system prompt including:
      * Role definition (2-3 sentences establishing expertise)
      * FOCUS AREAS section with bullet points
      * YOUR TONE section describing communication style
      * CONFIDENCE GUIDANCE section with high/medium/low ranges
      * Instructions to be precise and only report genuine issues

    The system prompt should be comprehensive and ready to use.

    {{ ctx.output_format }}
  "#
}

// Generate a complete configuration from requirements
function GenerateConfig(
  requirements: string,
  preferred_provider: string
) -> GeneratedConfig {
  client OpenRouter
  prompt #"
    You are an expert at configuring AI-powered code analysis tools.

    Create a complete gavel configuration based on the user's requirements. The config should:
    1. Choose an appropriate provider based on their needs
    2. Select sensible default policies for their use case
    3. Set appropriate defaults for their environment

    User's requirements: {{ requirements }}
    Preferred provider (if any): {{ preferred_provider }}

    Provider recommendations:
    - ollama: Free, local, requires Ollama running
    - openrouter: Pay-per-use, unified API, requires OPENROUTER_API_KEY
    - anthropic: Official Claude API, requires ANTHROPIC_API_KEY
    - bedrock: AWS-hosted, requires AWS credentials
    - openai: GPT models, requires OPENAI_API_KEY

    Model recommendations by provider:
    - ollama: "qwen2.5-coder:7b" (fast) or "gpt-oss:20b" (better quality)
    - openrouter: "anthropic/claude-haiku-4-5" (fast) or "anthropic/claude-sonnet-4" (quality)
    - anthropic: "claude-haiku-4-5" (fast) or "claude-sonnet-4" (quality)
    - bedrock: "anthropic.claude-haiku-4-5-v1:0" (fast) or "anthropic.claude-sonnet-4-v1:0" (quality)
    - openai: "o3-mini" (fast) or "gpt-5.3-codex" (quality)

    Generate a complete configuration with:
    - provider: Appropriate provider config with sensible defaults
    - persona: Default persona (code-reviewer, architect, or security)
    - policies: 2-4 relevant policies based on requirements

    {{ ctx.output_format }}
  "#
}
