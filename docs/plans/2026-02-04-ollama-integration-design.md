# Ollama Integration Design

**Date:** 2026-02-04
**Status:** Approved
**Default Model:** `gpt-oss:20b`

## Overview

This design adds Ollama support to Gavel, allowing users to run code analysis with local LLMs via Ollama instead of requiring OpenRouter API access. Users will configure their preferred provider in `.gavel/policies.yaml`, and Gavel will use the appropriate client at runtime.

## Goals

- Enable local LLM analysis via Ollama
- Maintain OpenRouter as default for existing users
- Use configuration-based provider selection (not CLI flags or env vars)
- Fail fast when configured provider is unavailable (no automatic fallback)
- Keep existing BAML architecture and interfaces unchanged

## Configuration Schema

### YAML Structure

Extend `.gavel/policies.yaml` with a top-level `provider` section:

```yaml
provider:
  name: ollama                    # "ollama" or "openrouter"
  ollama:
    model: gpt-oss:20b           # Ollama model name
    base_url: http://localhost:11434  # Optional, defaults to localhost:11434
  openrouter:
    model: anthropic/claude-sonnet-4  # OpenRouter model
    # API key still from OPENROUTER_API_KEY env var

policies:
  shall-be-merged:
    # ... existing policy config
```

### Configuration Merging

Provider configuration follows the same tiered merging as policies:

1. System defaults (embedded in binary)
2. Machine config (`~/.config/gavel/policies.yaml`)
3. Project config (`.gavel/policies.yaml`)

### System Defaults

```yaml
provider:
  name: openrouter
  ollama:
    model: gpt-oss:20b
    base_url: http://localhost:11434
  openrouter:
    model: anthropic/claude-sonnet-4
```

### Validation

- `provider.name` must be either `"ollama"` or `"openrouter"`
- If `provider.name = "ollama"`, `provider.ollama.model` must be set
- If `provider.name = "openrouter"`, `OPENROUTER_API_KEY` env var must be set
- Validation fails fast during config load if requirements not met

## BAML Changes

### Client Definitions

Update `baml_src/clients.baml` to define both clients:

```baml
// OpenRouter client for gavel analysis
client<llm> OpenRouter {
  provider "openai-generic"
  retry_policy Exponential
  options {
    base_url "https://openrouter.ai/api/v1"
    api_key env.OPENROUTER_API_KEY
    model "anthropic/claude-sonnet-4"
  }
}

// Ollama client for local LLM analysis
client<llm> Ollama {
  provider "openai-generic"
  retry_policy Exponential
  options {
    base_url "http://localhost:11434/v1"
    model "gpt-oss:20b"
  }
}

retry_policy Exponential {
  max_retries 2
  strategy {
    type exponential_backoff
    delay_ms 300
    multiplier 1.5
    max_delay_ms 10000
  }
}
```

**Design Notes:**
- Both clients use `openai-generic` provider (Ollama exposes OpenAI-compatible API at `/v1`)
- Shared retry policy ensures consistent behavior
- Default values are placeholders; runtime config overrides them
- No API key for Ollama (assumes local deployment without auth)

### Function Definition

The `AnalyzeCode` function in `baml_src/analyze.baml` remains unchanged:

```baml
function AnalyzeCode(code: string, policies: string) -> Finding[] {
  client OpenRouter  // Default for BAML generation
  prompt #"
    You are a precise code analyzer. Analyze the following code against the given policies.
    For each policy violation found, produce a finding. If no violations are found
    for a policy, do not produce a finding for it.

    Be precise about line numbers. Be concise in messages. Be thorough in explanations.
    Set confidence based on how certain you are — use lower confidence for ambiguous cases.

    Only report genuine issues. Do not fabricate findings.

    Policies:
    ---
    {{ policies }}
    ---

    Code:
    ---
    {{ code }}
    ---

    {{ ctx.output_format }}
  "#
}
```

**Design Notes:**
- Function signature unchanged (no breaking changes)
- `client OpenRouter` is the default for BAML code generation
- Actual client selection happens at runtime in Go wrapper

## Go Implementation

### Configuration Structs

Extend `internal/config/config.go`:

```go
// Config represents the merged gavel configuration
type Config struct {
    Provider ProviderConfig         `yaml:"provider"`
    Policies map[string]PolicyConfig `yaml:"policies"`
}

// ProviderConfig specifies which LLM provider to use
type ProviderConfig struct {
    Name       string              `yaml:"name"`        // "ollama" or "openrouter"
    Ollama     OllamaConfig        `yaml:"ollama"`
    OpenRouter OpenRouterConfig    `yaml:"openrouter"`
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
```

**Validation Logic:**

```go
func (c *Config) Validate() error {
    if c.Provider.Name != "ollama" && c.Provider.Name != "openrouter" {
        return fmt.Errorf("provider.name must be 'ollama' or 'openrouter', got: %s", c.Provider.Name)
    }

    if c.Provider.Name == "ollama" && c.Provider.Ollama.Model == "" {
        return fmt.Errorf("provider.ollama.model is required when using Ollama")
    }

    if c.Provider.Name == "openrouter" && os.Getenv("OPENROUTER_API_KEY") == "" {
        return fmt.Errorf("OPENROUTER_API_KEY environment variable required for OpenRouter")
    }

    return nil
}
```

### Runtime Client Selection

Update `internal/analyzer/bamlclient.go`:

```go
package analyzer

import (
    "context"
    "fmt"

    baml_client "github.com/chris-regnier/gavel/baml_client"
    "github.com/chris-regnier/gavel/baml_client/types"
    "github.com/chris-regnier/gavel/internal/config"
)

type BAMLLiveClient struct {
    config config.ProviderConfig
}

func NewBAMLLiveClient(cfg config.ProviderConfig) *BAMLLiveClient {
    return &BAMLLiveClient{config: cfg}
}

func (c *BAMLLiveClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]Finding, error) {
    var results []types.Finding
    var err error

    switch c.config.Name {
    case "ollama":
        results, err = c.analyzeWithOllama(ctx, code, policies)
    case "openrouter":
        results, err = c.analyzeWithOpenRouter(ctx, code, policies)
    default:
        return nil, fmt.Errorf("unknown provider: %s", c.config.Name)
    }

    if err != nil {
        return nil, fmt.Errorf("analysis failed with %s: %w", c.config.Name, err)
    }

    return convertFindings(results), nil
}

func (c *BAMLLiveClient) analyzeWithOllama(ctx context.Context, code string, policies string) ([]types.Finding, error) {
    // Call BAML-generated Ollama client
    // Note: May need to set runtime options based on c.config.Ollama
    return baml_client.AnalyzeCode(ctx, code, policies)
}

func (c *BAMLLiveClient) analyzeWithOpenRouter(ctx context.Context, code string, policies string) ([]types.Finding, error) {
    // Call BAML-generated OpenRouter client
    // Note: May need to set runtime options based on c.config.OpenRouter
    return baml_client.AnalyzeCode(ctx, code, policies)
}
```

**Design Notes:**
- Client constructor takes `ProviderConfig` from loaded config
- Switch statement dispatches to appropriate BAML-generated client
- Fail fast on unknown provider or connection errors
- No automatic fallback between providers
- Error messages include provider name for debugging

**Open Question:** BAML's generated client may not support runtime option overrides (model, base_url). If so, we'll need to:
- Generate wrapper functions that construct client-specific contexts
- OR use BAML's dynamic configuration API (if available)
- OR generate separate BAML projects per client (more complex)

This will be resolved during implementation.

### Integration Point

Update `cmd/gavel/analyze.go`:

```go
// Load config
cfg, err := config.LoadMerged(policyDir)
if err != nil {
    return fmt.Errorf("load config: %w", err)
}

// Validate config
if err := cfg.Validate(); err != nil {
    return fmt.Errorf("invalid config: %w", err)
}

// Create analyzer with provider config
client := analyzer.NewBAMLLiveClient(cfg.Provider)
analyzer := analyzer.New(client)
```

## Error Handling

### Fail-Fast Behavior

When `provider.name = "ollama"` is configured:

1. **Ollama not running:** Connection error returned immediately, analysis aborted
2. **Model not available:** BAML client returns error, analysis aborted
3. **Invalid base_url:** Connection error returned immediately

**No fallback to OpenRouter.** Users must explicitly change config to switch providers.

### Error Messages

Errors include provider context:

```
Error: analysis failed with ollama: connection refused to http://localhost:11434/v1
Error: invalid config: provider.ollama.model is required when using Ollama
Error: analysis failed with openrouter: OPENROUTER_API_KEY not set
```

## Testing Strategy

### Unit Tests

1. **Config loading** (`internal/config/config_test.go`):
   - Load YAML with provider settings
   - Validate tiered merging for provider config
   - Test validation errors (missing model, invalid provider name)

2. **Client selection** (`internal/analyzer/bamlclient_test.go`):
   - Mock switch logic in `AnalyzeCode()`
   - Verify correct client called based on config
   - Test error propagation

### Integration Tests

1. **OpenRouter integration** (existing):
   - Verify existing OpenRouter behavior unchanged
   - Requires `OPENROUTER_API_KEY`

2. **Ollama integration** (new):
   - Skip test if Ollama not running (`t.Skip()`)
   - Verify analysis works with Ollama client
   - Test fail-fast when Ollama unavailable

### Test Configuration

Example `.gavel/policies.yaml` for tests:

```yaml
provider:
  name: ollama
  ollama:
    model: llama3.2:1b  # Small model for CI
    base_url: http://localhost:11434

policies:
  shall-be-merged:
    enabled: true
    severity: error
```

## Documentation Updates

### README.md

Add Ollama setup section:

```markdown
### Using Ollama (Local LLMs)

Gavel supports local LLM analysis via [Ollama](https://ollama.ai/):

1. Install and start Ollama:
   ```bash
   brew install ollama
   ollama serve
   ```

2. Pull a model:
   ```bash
   ollama pull gpt-oss:20b
   ```

3. Configure Gavel to use Ollama in `.gavel/policies.yaml`:
   ```yaml
   provider:
     name: ollama
     ollama:
       model: gpt-oss:20b
       base_url: http://localhost:11434  # optional, this is the default
   ```

4. Run analysis:
   ```bash
   ./gavel analyze --dir ./src
   ```

**Switching back to OpenRouter:**

Change `provider.name` to `openrouter` and set `OPENROUTER_API_KEY`.
```

### CLAUDE.md

Update BAML section:

```markdown
## BAML

Source templates live in `baml_src/`. Generated Go client is in `baml_client/` (do not edit).

Gavel supports two LLM providers:
- **OpenRouter** (default): Requires `OPENROUTER_API_KEY` env var
- **Ollama** (local): Requires Ollama running at configured base_url

Provider selection is configured in `.gavel/policies.yaml` via the `provider` section. The BAML client wrapper (`internal/analyzer/bamlclient.go`) dispatches to the appropriate generated client based on this config.

After changing `.baml` files, run `task generate`.
```

## Implementation Checklist

- [ ] **BAML Changes:**
  - [ ] Add `Ollama` client definition to `baml_src/clients.baml`
  - [ ] Run `task generate` to regenerate Go client
  - [ ] Verify generated client includes both Ollama and OpenRouter paths

- [ ] **Configuration:**
  - [ ] Add `ProviderConfig`, `OllamaConfig`, `OpenRouterConfig` to `internal/config/config.go`
  - [ ] Update system defaults with provider config
  - [ ] Implement `Validate()` method on `Config`
  - [ ] Extend tiered merging to handle provider fields
  - [ ] Unit tests for config loading and validation

- [ ] **Client Selection:**
  - [ ] Update `NewBAMLLiveClient` signature to accept `ProviderConfig`
  - [ ] Implement `analyzeWithOllama()` method
  - [ ] Implement `analyzeWithOpenRouter()` method
  - [ ] Add switch logic in `AnalyzeCode()` based on `config.Name`
  - [ ] Unit tests for client selection (with mocks)

- [ ] **Integration:**
  - [ ] Update `cmd/gavel/analyze.go` to pass provider config to analyzer
  - [ ] Add config validation call before analysis
  - [ ] Integration test with Ollama (skipped if not running)
  - [ ] Integration test with OpenRouter (existing)

- [ ] **Documentation:**
  - [ ] Update README.md with Ollama setup instructions
  - [ ] Add example `.gavel/policies.yaml` with both providers
  - [ ] Update CLAUDE.md with new architecture details
  - [ ] Add inline code comments for provider selection logic

- [ ] **Final Verification:**
  - [ ] `task lint` passes
  - [ ] `task test` passes
  - [ ] `task build` produces working binary
  - [ ] Manual test with Ollama
  - [ ] Manual test with OpenRouter

## Trade-offs and Decisions

### Why Configuration Over CLI Flags?

**Decision:** Use `.gavel/policies.yaml` instead of `--provider` CLI flag.

**Rationale:**
- Provider choice is project-specific (CI vs local dev)
- Tiered config allows machine-level defaults
- Consistent with existing policy configuration pattern
- Reduces CLI complexity

**Trade-off:** Switching providers requires config edit instead of flag change. Acceptable because provider changes are infrequent.

### Why Fail-Fast Instead of Fallback?

**Decision:** Return error immediately if configured provider unavailable.

**Rationale:**
- Explicit > implicit (user knows which LLM analyzed their code)
- Prevents silent degradation (e.g., Ollama down → expensive OpenRouter calls)
- Simpler error handling (no fallback chain logic)

**Trade-off:** Less convenient for users who want "just work" behavior. Acceptable because provider reliability is critical for consistent analysis.

### Why Both Clients in Same BAML Project?

**Decision:** Define both `Ollama` and `OpenRouter` clients in `baml_src/clients.baml`.

**Rationale:**
- Single `task generate` command
- Shared prompt templates and types
- Runtime selection in Go wrapper is simple

**Trade-off:** BAML generates code for both clients even if only one is used. Acceptable because code size is negligible and simplifies build process.

## Future Enhancements (Out of Scope)

- **Multiple model support:** Allow different models per policy (e.g., fast model for `function-length`, powerful model for `shall-be-merged`)
- **Automatic model detection:** Query Ollama for available models and validate config
- **Provider-specific prompt tuning:** Adjust prompts based on model capabilities
- **Cost tracking:** Track OpenRouter API costs, compare with Ollama usage
- **Parallel analysis:** Use both providers and compare results for confidence scoring

These are explicitly **not** part of this design. They can be added in future iterations if needed.
