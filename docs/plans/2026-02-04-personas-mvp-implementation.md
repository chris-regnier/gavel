# Personas MVP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add switchable AI personas (code-reviewer, architect, security) that analyze code from different expert perspectives using BAML prompt engineering.

**Architecture:** BAML defines persona prompt functions, Go config system allows persona selection, analyzer injects persona prompt into LLM analysis. All personas use same `Finding[]` output format with consistent SARIF metadata.

**Tech Stack:** Go 1.25+, BAML CLI, YAML config, SARIF 2.1.0

---

## Task 1: Create BAML Persona Prompts

**Files:**
- Create: `baml_src/personas.baml`

**Step 1: Create personas.baml with three persona prompt functions**

```baml
// Persona prompt generators for Gavel analysis
// Each persona represents a different expert perspective

function GetCodeReviewerPrompt() -> string {
  client OpenRouter
  prompt #"
    Return this exact text:

    You are a senior code reviewer with 15 years of experience in software engineering.
    Your role is to ensure code quality, maintainability, and adherence to best practices.

    FOCUS ON:
    - Code clarity and readability - can future maintainers understand this easily?
    - Proper error handling and edge cases - what could go wrong?
    - Test coverage and testability - is this code designed to be tested?
    - Design patterns and SOLID principles - is the code well-structured?
    - Performance implications - are there obvious inefficiencies?

    TONE: Constructive and educational. Always explain the "why" behind suggestions.
    Think of yourself as mentoring a junior developer - be specific and helpful.

    CONFIDENCE GUIDANCE:
    - High confidence (0.8-1.0): Clear violations of best practices, obvious bugs
    - Medium confidence (0.5-0.8): Code smells, potential issues, subjective improvements
    - Low confidence (0.0-0.5): Suggestions, alternative approaches, minor style issues
  "#
}

function GetArchitectPrompt() -> string {
  client OpenRouter
  prompt #"
    Return this exact text:

    You are a system architect specializing in distributed systems and scalable design.
    Your role is to evaluate code from a high-level architectural perspective.

    FOCUS ON:
    - Service boundaries and separation of concerns - is this in the right module?
    - API design and interface contracts - will this API be easy to use and maintain?
    - Scalability and performance at scale - how will this behave with 10x load?
    - Integration patterns and dependencies - how does this fit in the system?
    - System-wide consistency and standards - does this follow our patterns?

    TONE: Strategic and forward-thinking. Consider long-term implications and technical debt.
    Think about the system as a whole, not just the immediate code.

    CONFIDENCE GUIDANCE:
    - High confidence (0.8-1.0): Architectural anti-patterns, tight coupling, poor boundaries
    - Medium confidence (0.5-0.8): Questionable design choices, scalability concerns
    - Low confidence (0.0-0.5): Alternative approaches, optimization opportunities
  "#
}

function GetSecurityPrompt() -> string {
  client OpenRouter
  prompt #"
    Return this exact text:

    You are a security engineer conducting a thorough code security review.
    Your role is to identify vulnerabilities, security risks, and potential attack vectors.

    FOCUS ON:
    - OWASP Top 10 vulnerabilities - injection, broken auth, XSS, etc.
    - Input validation and sanitization - is all external input validated?
    - Authentication and authorization flows - who can access what?
    - Secrets management and credential handling - are credentials secure?
    - Dependency security and supply chain risks - are dependencies trustworthy?
    - Attack surface and threat vectors - what could an attacker exploit?

    TONE: Direct and risk-focused. Prioritize findings by severity and exploitability.
    Think like an attacker - how would you break this code?

    CONFIDENCE GUIDANCE:
    - High confidence (0.8-1.0): Known vulnerabilities, direct exploits, security failures
    - Medium confidence (0.5-0.8): Potential vulnerabilities, security smells, risky patterns
    - Low confidence (0.0-0.5): Hardening opportunities, defense-in-depth suggestions
  "#
}
```

**Step 2: Verify file created**

Run: `ls -l baml_src/personas.baml`
Expected: File exists with ~100 lines

**Step 3: Commit**

```bash
git add baml_src/personas.baml
git commit -m "feat(baml): add three persona prompt functions

- GetCodeReviewerPrompt: quality, error handling, testability
- GetArchitectPrompt: scalability, API design, boundaries
- GetSecurityPrompt: OWASP Top 10, auth/authz, injection

Each persona has specific focus areas, tone guidance, and confidence levels."
```

---

## Task 2: Update BAML AnalyzeCode Function

**Files:**
- Modify: `baml_src/analyze.baml:10-22`

**Step 1: Read current AnalyzeCode function**

Run: `cat baml_src/analyze.baml`
Expected: Function signature `AnalyzeCode(code: string, policies: string)`

**Step 2: Update function to accept persona prompt and additional context**

Replace the function definition with:

```baml
function AnalyzeCode(
  code: string,
  policies: string,
  personaPrompt: string,
  additionalContext: string
) -> Finding[] {
  client OpenRouter
  prompt #"
    {{ personaPrompt }}

    {% if additionalContext != "" %}
    ===== ADDITIONAL CONTEXT =====
    The following context may be relevant to your analysis:

    {{ additionalContext }}

    ===== END CONTEXT =====
    {% endif %}

    ===== POLICIES TO CHECK =====
    Analyze the code against these specific policies. Only report genuine violations.
    If a policy doesn't apply to this code, don't force a finding.

    {{ policies }}

    ===== CODE TO ANALYZE =====
    {{ code }}

    ===== INSTRUCTIONS =====
    For each policy violation or issue you find:
    1. Identify the exact line numbers where it occurs
    2. Write a concise message (one sentence)
    3. Provide a detailed explanation following your persona's tone
    4. Suggest a specific, actionable recommendation
    5. Assign an appropriate confidence level based on the guidance above

    Only report genuine issues. Quality over quantity.

    {{ ctx.output_format }}
  "#
}
```

**Step 3: Verify changes**

Run: `grep -A 5 "function AnalyzeCode" baml_src/analyze.baml`
Expected: Shows four parameters including `personaPrompt` and `additionalContext`

**Step 4: Commit**

```bash
git add baml_src/analyze.baml
git commit -m "feat(baml): add personaPrompt and additionalContext parameters to AnalyzeCode

- personaPrompt: injects expert perspective from persona functions
- additionalContext: optional context like docs or related files
- Prompt restructured with clear sections for persona, context, policies, code"
```

---

## Task 3: Regenerate BAML Client

**Files:**
- Modified: `baml_client/` (generated, many files)

**Step 1: Run BAML code generation**

Run: `task generate`
Expected: Output shows "Successfully generated client" or similar

**Step 2: Verify new functions exist**

Run: `grep -r "GetCodeReviewerPrompt" baml_client/`
Expected: Function exists in generated Go code

**Step 3: Verify AnalyzeCode signature updated**

Run: `grep -A 3 "func AnalyzeCode" baml_client/client.go`
Expected: Function has four parameters now

**Step 4: Commit generated code**

```bash
git add baml_client/
git commit -m "chore(baml): regenerate client with persona functions

Generated from baml_src changes:
- Three persona prompt functions
- Updated AnalyzeCode with personaPrompt and additionalContext params"
```

---

## Task 4: Add Persona Field to Config

**Files:**
- Modify: `internal/config/config.go:27-29`
- Modify: `internal/config/config.go:47-60`

**Step 1: Add Persona field to Config struct**

In `internal/config/config.go`, update the `Config` struct:

```go
// Config holds the full gavel configuration.
type Config struct {
	Provider ProviderConfig    `yaml:"provider"`
	Persona  string            `yaml:"persona"` // NEW: AI expert role (code-reviewer, architect, security)
	Policies map[string]Policy `yaml:"policies"`
}
```

**Step 2: Add persona validation to Validate method**

Add validation logic in the `Validate()` method after provider validation:

```go
func (c *Config) Validate() error {
	// ... existing provider validation

	// Validate persona if specified
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
```

**Step 3: Run tests to check compilation**

Run: `go test ./internal/config/ -v`
Expected: Tests compile (may fail, we'll fix tests next)

**Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add Persona field with validation

- Persona string field in Config struct
- Validates against three valid personas: code-reviewer, architect, security
- Empty persona is valid (will use default)"
```

---

## Task 5: Update Config Defaults

**Files:**
- Modify: `internal/config/defaults.go:5-8`

**Step 1: Add default persona to SystemDefaults**

Update `SystemDefaults()` function to include default persona:

```go
func SystemDefaults() *Config {
	return &Config{
		Provider: ProviderConfig{
			Name: "openrouter",
			Ollama: OllamaConfig{
				Model:   "gpt-oss:20b",
				BaseURL: "http://localhost:11434/v1",
			},
			OpenRouter: OpenRouterConfig{
				Model: "anthropic/claude-sonnet-4",
			},
		},
		Persona: "code-reviewer", // NEW: default persona
		Policies: map[string]Policy{
			// ... existing policies
		},
	}
}
```

**Step 2: Verify defaults**

Run: `go test ./internal/config/ -run TestSystemDefaults -v`
Expected: Test compiles and runs

**Step 3: Commit**

```bash
git add internal/config/defaults.go
git commit -m "feat(config): set code-reviewer as default persona

Default provides balanced code quality analysis suitable for daily PR reviews."
```

---

## Task 6: Update Config Merging

**Files:**
- Modify: `internal/config/config.go:83-98`

**Step 1: Add persona merging logic**

In `MergeConfigs()` function, add persona merging after provider merging:

```go
func MergeConfigs(configs ...*Config) *Config {
	result := &Config{
		Policies: make(map[string]Policy),
	}

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		// Merge provider config - non-empty string fields override
		// ... existing provider merging code

		// NEW: Merge persona (non-empty overrides)
		if cfg.Persona != "" {
			result.Persona = cfg.Persona
		}

		// Merge policies (existing logic)
		// ... existing policy merging code
	}

	return result
}
```

**Step 2: Run merge tests**

Run: `go test ./internal/config/ -run TestMerge -v`
Expected: All merge tests pass

**Step 3: Commit**

```bash
git add internal/config/config.go
git commit -m "feat(config): add persona to config merging logic

Non-empty persona values from higher-tier configs override lower tiers.
Follows same pattern as provider config merging."
```

---

## Task 7: Write Config Tests for Persona

**Files:**
- Create: `internal/config/config_test.go` (add test function)

**Step 1: Write test for persona validation**

Add test function to `internal/config/config_test.go`:

```go
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
				Provider: ProviderConfig{Name: "openrouter"},
				Persona:  tt.persona,
			}

			err := cfg.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown persona")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestConfigValidation_Persona -v`
Expected: PASS with 5 test cases

**Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test(config): add persona validation tests

Tests validate:
- Three valid personas accepted
- Invalid persona rejected with error
- Empty persona allowed (uses default)"
```

---

## Task 8: Create Persona Selector

**Files:**
- Create: `internal/analyzer/personas.go`

**Step 1: Create personas.go with GetPersonaPrompt function**

```go
package analyzer

import (
	"context"
	"fmt"

	baml_client "github.com/chris-regnier/gavel/baml_client"
)

// GetPersonaPrompt returns the BAML-generated prompt for the given persona.
// Valid personas: code-reviewer, architect, security
func GetPersonaPrompt(ctx context.Context, persona string) (string, error) {
	switch persona {
	case "code-reviewer":
		return baml_client.GetCodeReviewerPrompt(ctx)
	case "architect":
		return baml_client.GetArchitectPrompt(ctx)
	case "security":
		return baml_client.GetSecurityPrompt(ctx)
	default:
		return "", fmt.Errorf("unknown persona: %s", persona)
	}
}
```

**Step 2: Verify file compiles**

Run: `go build ./internal/analyzer/`
Expected: No compilation errors

**Step 3: Commit**

```bash
git add internal/analyzer/personas.go
git commit -m "feat(analyzer): add GetPersonaPrompt selector function

Maps persona string to BAML-generated prompt function.
Provides single entry point for persona selection logic."
```

---

## Task 9: Write Persona Selector Tests

**Files:**
- Create: `internal/analyzer/personas_test.go`

**Step 1: Create test file with persona prompt tests**

```go
package analyzer_test

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/stretchr/testify/assert"
)

func TestGetPersonaPrompt(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		persona     string
		shouldExist string
		wantErr     bool
	}{
		{"code-reviewer", "senior code reviewer", false},
		{"architect", "system architect", false},
		{"security", "security engineer", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.persona, func(t *testing.T) {
			prompt, err := analyzer.GetPersonaPrompt(ctx, tt.persona)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown persona")
				return
			}

			assert.NoError(t, err)
			assert.NotEmpty(t, prompt)
			assert.Contains(t, prompt, tt.shouldExist)
		})
	}
}
```

**Step 2: Run test to verify BAML functions work**

Run: `go test ./internal/analyzer/ -run TestGetPersonaPrompt -v`
Expected: PASS with 4 test cases

**Step 3: Commit**

```bash
git add internal/analyzer/personas_test.go
git commit -m "test(analyzer): add persona prompt retrieval tests

Tests verify:
- Three valid personas return expected prompts
- Invalid persona returns error
- BAML generated functions work correctly"
```

---

## Task 10: Update BAMLClient Interface

**Files:**
- Modify: `internal/analyzer/analyzer.go:13-20`

**Step 1: Update BAMLClient interface to include new parameters**

```go
// BAMLClient defines the interface for LLM-based code analysis.
type BAMLClient interface {
	AnalyzeCode(
		ctx context.Context,
		code string,
		policies string,
		personaPrompt string,     // NEW: Persona's expert perspective
		additionalContext string, // NEW: Optional context (empty string if none)
	) ([]Finding, error)
}
```

**Step 2: Verify interface compiles**

Run: `go build ./internal/analyzer/`
Expected: Compilation errors (implementations need updating - we'll fix next)

**Step 3: Commit interface change**

```bash
git add internal/analyzer/analyzer.go
git commit -m "feat(analyzer): update BAMLClient interface with persona params

Adds personaPrompt and additionalContext parameters to AnalyzeCode method.
Matches updated BAML function signature."
```

---

## Task 11: Update BAMLLiveClient Implementation

**Files:**
- Modify: `internal/analyzer/bamlclient.go:25-48`

**Step 1: Update AnalyzeCode method signature and implementation**

```go
// AnalyzeCode calls the appropriate BAML client based on provider config.
func (c *BAMLLiveClient) AnalyzeCode(
	ctx context.Context,
	code string,
	policies string,
	personaPrompt string,     // NEW
	additionalContext string, // NEW
) ([]Finding, error) {
	var results []types.Finding
	var err error

	switch c.providerConfig.Name {
	case "ollama":
		env := map[string]string{
			"OLLAMA_MODEL": c.providerConfig.Ollama.Model,
		}
		if c.providerConfig.Ollama.BaseURL != "" {
			env["OLLAMA_BASE_URL"] = c.providerConfig.Ollama.BaseURL
		} else {
			env["OLLAMA_BASE_URL"] = "http://localhost:11434/v1"
		}
		results, err = baml_client.AnalyzeCode(
			ctx,
			code,
			policies,
			personaPrompt,     // NEW
			additionalContext, // NEW
			baml_client.WithClient("Ollama"),
			baml_client.WithEnv(env),
		)
	case "openrouter":
		results, err = baml_client.AnalyzeCode(
			ctx,
			code,
			policies,
			personaPrompt,     // NEW
			additionalContext, // NEW
			baml_client.WithClient("OpenRouter"),
		)
	default:
		return nil, fmt.Errorf("unknown provider: %s", c.providerConfig.Name)
	}

	if err != nil {
		return nil, fmt.Errorf("analysis failed with %s: %w", c.providerConfig.Name, err)
	}

	return convertFindings(results), nil
}
```

**Step 2: Verify compilation**

Run: `go build ./internal/analyzer/`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add internal/analyzer/bamlclient.go
git commit -m "feat(analyzer): update BAMLLiveClient with persona parameters

Passes personaPrompt and additionalContext to BAML AnalyzeCode for both
Ollama and OpenRouter providers."
```

---

## Task 12: Update Mock Client

**Files:**
- Modify: `internal/analyzer/mock.go:8-21`

**Step 1: Update MockBAMLClient to match new interface**

```go
// MockBAMLClient implements BAMLClient for testing
type MockBAMLClient struct {
	AnalyzeCodeFunc func(
		ctx context.Context,
		code, policies, personaPrompt, additionalContext string, // NEW params
	) ([]Finding, error)
}

func (m *MockBAMLClient) AnalyzeCode(
	ctx context.Context,
	code, policies, personaPrompt, additionalContext string, // NEW params
) ([]Finding, error) {
	if m.AnalyzeCodeFunc != nil {
		return m.AnalyzeCodeFunc(ctx, code, policies, personaPrompt, additionalContext)
	}
	return []Finding{}, nil
}
```

**Step 2: Fix existing tests that use mock**

Run: `go test ./internal/analyzer/ -v`
Expected: Tests compile (may fail, need to update test calls)

**Step 3: Update test calls to include new parameters**

Find and update test calls to mock in `internal/analyzer/*_test.go`:

```go
// Example update
findings, err := mock.AnalyzeCode(
	context.Background(),
	"code",
	"policies",
	"You are a code reviewer", // NEW: persona prompt
	"",                         // NEW: additional context (empty)
)
```

**Step 4: Run tests again**

Run: `go test ./internal/analyzer/ -v`
Expected: All tests pass

**Step 5: Commit**

```bash
git add internal/analyzer/mock.go internal/analyzer/*_test.go
git commit -m "test(analyzer): update mock client and tests with persona params

MockBAMLClient signature matches updated interface.
All existing tests updated to pass empty persona/context."
```

---

## Task 13: Add Persona Flag to CLI

**Files:**
- Modify: `cmd/gavel/root.go:35-42`

**Step 1: Add --persona flag to root command**

In `init()` function of `root.go`:

```go
func init() {
	// ... existing flags

	// Persona flag for runtime persona override
	rootCmd.PersistentFlags().String(
		"persona",
		"",
		"Persona to use for analysis (code-reviewer, architect, security). Overrides config.",
	)
}
```

**Step 2: Verify flag appears in help**

Run: `go build ./cmd/gavel/ && ./gavel --help`
Expected: Shows `--persona` flag in output

**Step 3: Commit**

```bash
git add cmd/gavel/root.go
git commit -m "feat(cli): add --persona flag for runtime persona selection

Allows overriding configured persona from command line.
Useful for one-off security audits or architecture reviews."
```

---

## Task 14: Integrate Persona in Analyze Command (Part 1: Loading)

**Files:**
- Modify: `cmd/gavel/analyze.go:45-65`

**Step 1: Add persona override from CLI flag**

After loading config in `runAnalyze()`:

```go
func runAnalyze(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// ... existing config loading code

	// Override persona from CLI flag if provided
	if personaFlag, _ := cmd.Flags().GetString("persona"); personaFlag != "" {
		cfg.Persona = personaFlag
	}

	// Validate config including persona
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// ... rest of function
}
```

**Step 2: Verify flag override works**

Run: `go build ./cmd/gavel/`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add cmd/gavel/analyze.go
git commit -m "feat(analyze): add CLI persona flag override

CLI --persona flag overrides config persona.
Validation ensures only valid personas accepted."
```

---

## Task 15: Integrate Persona in Analyze Command (Part 2: Prompt Loading)

**Files:**
- Modify: `cmd/gavel/analyze.go:68-75`

**Step 1: Load persona prompt from BAML**

After config validation in `runAnalyze()`:

```go
	// ... after config validation

	// Get persona prompt from BAML
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, cfg.Persona)
	if err != nil {
		return fmt.Errorf("loading persona %s: %w", cfg.Persona, err)
	}

	// ... continue with input handling
```

**Step 2: Verify compilation**

Run: `go build ./cmd/gavel/`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add cmd/gavel/analyze.go
git commit -m "feat(analyze): load persona prompt from BAML

Retrieves configured persona's prompt before analysis begins.
Fails fast with clear error if persona invalid."
```

---

## Task 16: Integrate Persona in Analyze Command (Part 3: Analysis Call)

**Files:**
- Modify: `cmd/gavel/analyze.go:120-135`

**Step 1: Update AnalyzeCode calls to include persona prompt**

In the artifact analysis loop:

```go
	// Analyze each artifact
	for _, artifact := range artifacts {
		// MVP: Empty additional context (Phase 2 will add context selectors)
		additionalContext := ""

		// Call analyzer with persona prompt
		findings, err := client.AnalyzeCode(
			ctx,
			artifact.Content,
			policiesText,
			personaPrompt,     // NEW: persona's expert perspective
			additionalContext, // NEW: optional context (empty for MVP)
		)
		if err != nil {
			return fmt.Errorf("analyzing %s: %w", artifact.Path, err)
		}

		// ... rest of SARIF assembly
	}
```

**Step 2: Run build to check all integrations**

Run: `go build ./cmd/gavel/`
Expected: Builds successfully

**Step 3: Commit**

```bash
git add cmd/gavel/analyze.go
git commit -m "feat(analyze): pass persona prompt to analyzer

Every artifact analyzed with selected persona's perspective.
Additional context empty for MVP (future: context selectors)."
```

---

## Task 17: Add Persona to SARIF Metadata

**Files:**
- Modify: `internal/sarif/sarif.go:25-35`
- Modify: `internal/sarif/sarif.go:85-95`

**Step 1: Update NewRun to accept config and add persona metadata**

Update `NewRun()` signature and add persona to tool properties:

```go
func NewRun(findings []analyzer.Finding, cfg *config.Config) *Run {
	run := &Run{
		Tool: Tool{
			Driver: ToolComponent{
				Name:    "gavel",
				Version: version,
				Properties: map[string]interface{}{
					"gavel/persona": cfg.Persona, // NEW: track which persona was used
				},
			},
		},
		Results: []Result{},
	}

	// ... rest of function
}
```

**Step 2: Update convertFinding to add persona to individual findings**

```go
func convertFinding(f analyzer.Finding, cfg *config.Config) Result {
	result := Result{
		RuleID:  f.RuleID,
		Level:   f.Level,
		Message: Message{Text: f.Message},
		Locations: []Location{
			{
				PhysicalLocation: PhysicalLocation{
					ArtifactLocation: ArtifactLocation{URI: f.FilePath},
					Region: Region{
						StartLine: f.StartLine,
						EndLine:   f.EndLine,
					},
				},
			},
		},
		Properties: map[string]interface{}{
			"gavel/confidence":    f.Confidence,
			"gavel/explanation":   f.Explanation,
			"gavel/recommendation": f.Recommendation,
			"gavel/persona":       cfg.Persona, // NEW: track persona per finding
		},
	}
	return result
}
```

**Step 3: Update all NewRun calls to pass config**

Find calls to `NewRun()` in `cmd/gavel/analyze.go` and pass config:

```go
run := sarif.NewRun(allFindings, cfg) // NEW: pass config
```

**Step 4: Run tests**

Run: `go test ./internal/sarif/ -v`
Expected: May have compilation errors in tests - fix test calls

**Step 5: Fix test calls to pass config**

Update SARIF tests to pass config:

```go
cfg := &config.Config{Persona: "code-reviewer"}
run := sarif.NewRun(findings, cfg)
```

**Step 6: Verify tests pass**

Run: `go test ./internal/sarif/ -v`
Expected: All tests pass

**Step 7: Commit**

```bash
git add internal/sarif/sarif.go internal/sarif/*_test.go cmd/gavel/analyze.go
git commit -m "feat(sarif): add persona metadata to SARIF output

Tool properties include active persona.
Individual findings tagged with persona for filtering/analysis.
All SARIF tests updated to pass config."
```

---

## Task 18: Update Documentation

**Files:**
- Modify: `README.md:110-135`
- Modify: `CLAUDE.md:50-65`

**Step 1: Add Personas section to README**

After the "Using Ollama" section in README.md:

```markdown
## Personas

Gavel supports different analysis personas for specialized code review:

- `code-reviewer` (default): Focuses on code quality, bugs, and best practices
- `architect`: Focuses on system design, scalability, and API patterns
- `security`: Focuses on vulnerabilities and OWASP Top 10

### Using Personas

**Via config** (`.gavel/policies.yaml`):

\`\`\`yaml
persona: security
\`\`\`

**Via CLI flag**:

\`\`\`bash
gavel analyze --persona architect --dir ./src
\`\`\`

Different personas provide specialized expertise:
- Use `code-reviewer` for daily PR reviews
- Use `architect` for architecture reviews
- Use `security` for security audits before releases
```

**Step 2: Update CLAUDE.md with persona implementation details**

After the BAML section in CLAUDE.md:

```markdown
## Personas

Gavel uses BAML to implement switchable analysis personas. Different personas provide
specialized expert perspectives: code quality, architecture, or security.

**Implementation:**
- `baml_src/personas.baml` - Persona prompt definitions
- `internal/analyzer/personas.go` - Persona selection logic
- `internal/config/config.go` - Persona configuration field
- `docs/personas-feature-design.md` - Full design document

**To add a new persona:**
1. Add prompt function to `baml_src/personas.baml`
2. Add case to `GetPersonaPrompt()` in `internal/analyzer/personas.go`
3. Add to valid personas map in `internal/config/config.go` validation
4. Run `task generate` to regenerate BAML client
5. Update documentation

**Current personas:**
- `code-reviewer` (default): Code quality, error handling, testability
- `architect`: Scalability, API design, service boundaries
- `security`: OWASP Top 10, auth/authz, injection vulnerabilities
```

**Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: add personas feature documentation

README: user-facing persona usage guide
CLAUDE.md: developer implementation guide with examples"
```

---

## Task 19: Integration Test

**Files:**
- Create: `cmd/gavel/analyze_persona_test.go`

**Step 1: Write integration test for persona selection**

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPersonaIntegration(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create .gavel directory
	gavelDir := filepath.Join(tmpDir, ".gavel")
	require.NoError(t, os.MkdirAll(gavelDir, 0755))

	// Create config with security persona
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: "openrouter",
			OpenRouter: config.OpenRouterConfig{
				Model: "anthropic/claude-sonnet-4",
			},
		},
		Persona: "security",
		Policies: map[string]config.Policy{
			"shall-be-merged": {
				Description: "Test policy",
				Severity:    "error",
				Instruction: "Flag issues",
				Enabled:     true,
			},
		},
	}

	// Write config
	configPath := filepath.Join(gavelDir, "policies.yaml")
	configData, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, configData, 0644))

	// Load config
	loaded, err := config.LoadFromFile(configPath)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify persona loaded correctly
	assert.Equal(t, "security", loaded.Persona)

	// Validate config
	assert.NoError(t, loaded.Validate())

	// Test persona override via flag simulation
	loaded.Persona = "architect"
	assert.Equal(t, "architect", loaded.Persona)
	assert.NoError(t, loaded.Validate())

	// Test invalid persona
	loaded.Persona = "invalid"
	assert.Error(t, loaded.Validate())
	assert.Contains(t, loaded.Validate().Error(), "unknown persona")
}
```

**Step 2: Run integration test**

Run: `go test ./cmd/gavel/ -run TestPersonaIntegration -v`
Expected: PASS

**Step 3: Commit**

```bash
git add cmd/gavel/analyze_persona_test.go
git commit -m "test(integration): add persona config integration test

Tests:
- Config loading with persona
- Persona validation
- CLI flag override behavior
- Invalid persona rejection"
```

---

## Task 20: Full System Test

**Files:**
- Test: entire pipeline

**Step 1: Build gavel**

Run: `task build`
Expected: Builds successfully, creates `./gavel` binary

**Step 2: Create test config with persona**

```bash
mkdir -p .gavel
cat > .gavel/policies.yaml << 'EOF'
persona: security
policies:
  shall-be-merged:
    enabled: true
    severity: error
    instruction: "Flag security vulnerabilities"
EOF
```

**Step 3: Create test file with vulnerability**

```bash
mkdir -p test-code
cat > test-code/vulnerable.go << 'EOF'
package main

import "database/sql"

func GetUser(id string) {
    db.Query("SELECT * FROM users WHERE id = " + id)
}
EOF
```

**Step 4: Run analysis (if OPENROUTER_API_KEY available)**

Run: `./gavel analyze --dir ./test-code` (skip if no API key)
Expected: If run, creates SARIF output with persona metadata

**Step 5: Check SARIF output for persona metadata**

Run: `cat .gavel/results/*/sarif.json | jq '.runs[0].tool.driver.properties["gavel/persona"]'`
Expected: Shows `"security"` (if test ran)

**Step 6: Test CLI override**

Run: `./gavel analyze --persona architect --dir ./test-code` (skip if no API key)
Expected: If run, SARIF shows `"architect"`

**Step 7: Clean up test files**

```bash
rm -rf .gavel test-code
```

**Step 8: Run all tests**

Run: `task test`
Expected: All tests pass

**Step 9: Final commit**

```bash
git add .
git commit -m "test: verify full personas MVP pipeline

Manual testing confirms:
- Config persona selection works
- CLI flag override works
- SARIF metadata includes persona
- All unit and integration tests pass

MVP complete: 3 personas (code-reviewer, architect, security) functional."
```

---

## Verification Checklist

After completing all tasks:

- [ ] `task generate` completes without errors
- [ ] `task build` creates working `./gavel` binary
- [ ] `task test` all tests pass
- [ ] `task lint` no issues
- [ ] Can set persona in `.gavel/policies.yaml`
- [ ] Can override persona with `--persona` flag
- [ ] Invalid persona is rejected with error message
- [ ] SARIF output includes `gavel/persona` in tool properties
- [ ] SARIF findings include `gavel/persona` in properties
- [ ] README documents persona usage
- [ ] CLAUDE.md documents implementation

---

## Success Criteria

✅ **Functional Requirements:**
- Three personas implemented: code-reviewer, architect, security
- Persona selectable via config YAML
- Persona overridable via CLI flag
- BAML generates distinct prompts for each persona
- Invalid personas rejected with clear error

✅ **Technical Requirements:**
- All tests pass
- Code follows existing patterns
- SARIF metadata includes persona
- Documentation complete

✅ **Quality Requirements:**
- TDD approach with tests first where possible
- Frequent, logical commits
- No breaking changes to existing functionality
- Backward compatible (empty persona uses default)

---

## Future Enhancements (Not in MVP)

These are explicitly out of scope for MVP:

- **Phase 2**: Add technical-writer and performance personas
- **Phase 3**: Persona-specific context selectors
- **Phase 4**: Multi-persona analysis (run multiple personas in parallel)
- **Phase 5**: Custom user-defined personas
- **Phase 6**: Persona override patterns (different persona per file type)

MVP focuses on proving the core concept with 3 personas and basic functionality.
