# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
task build          # go build -o gavel ./cmd/gavel
task test           # go test ./... -v
task lint           # go vet ./...
task generate       # baml-cli generate (regenerates baml_client/ from baml_src/)

# Run a single test
go test ./internal/config/ -run TestMergeOverrides -v

# Run integration test only
go test -run TestIntegration -v

# Run the tool
OPENROUTER_API_KEY=... ./gavel analyze --dir ./internal/input
```

## Architecture

Gavel is an AI-powered code analysis CLI that gates CI workflows (auto-merge, reject, human review) by analyzing code against configurable policies via an LLM, producing SARIF output, and evaluating it with Rego.

**Pipeline:**
```
Input Handler → BAML Analyzer → SARIF Assembler → Rego Evaluator → Verdict
                                       ↓                ↓
                                 FileStore ←─────────────┘
```

**Data flow in `cmd/gavel/analyze.go`:**
1. Load tiered config (system defaults → `~/.config/gavel/policies.yaml` → `.gavel/policies.yaml`)
2. Read artifacts via input handler (files, unified diff, or directory walk)
3. Format enabled policies into text, call BAML `AnalyzeCode` per artifact
4. Convert findings to SARIF results with `gavel/` property extensions (recommendation, explanation, confidence)
5. Deduplicate overlapping findings, assemble SARIF 2.1.0 log
6. Store SARIF, evaluate with Rego, store verdict, output JSON

## Key Design Decisions

- **`BAMLClient` interface** (`internal/analyzer/analyzer.go`): All tests use a mock client. `BAMLLiveClient` (`bamlclient.go`) wraps the generated `baml_client.AnalyzeCode` function. The generated BAML types use `int64`/`RuleId`; the internal `Finding` type uses `int`/`RuleID`.
- **Tiered config merging** (`internal/config/config.go`): Non-zero string fields override; `Enabled` bool always applies. `LoadFromFile` returns nil/nil for missing files.
- **SARIF extensions**: All gavel-specific data lives in `Properties map[string]interface{}` with `gavel/` prefix keys.
- **Rego evaluator** (`internal/evaluator/evaluator.go`): Default policy is embedded via `//go:embed default.rego`. Custom `.rego` files from a directory override it. Rego receives the full SARIF log as JSON input; it never sees source code.
- **Storage** (`internal/store/`): `Store` interface with filesystem implementation. IDs are `<timestamp>-<hex>` directories under `.gavel/results/`.

## BAML

Source templates live in `baml_src/`. Generated Go client is in `baml_client/` (do not edit).

Gavel supports multiple LLM providers:

**Supported Providers:**
1. **Ollama** (local, free): Requires Ollama running at configured base_url (default: `http://localhost:11434/v1`), model `gpt-oss:20b`
2. **OpenRouter** (unified API): Requires `OPENROUTER_API_KEY` env var, model `anthropic/claude-sonnet-4`
3. **Anthropic** (direct API): Requires `ANTHROPIC_API_KEY` env var, supports all Claude models
4. **Bedrock** (AWS): Requires AWS credentials, supports Claude models on AWS Bedrock
5. **OpenAI** (direct API): Requires `OPENAI_API_KEY` env var, supports GPT-4 and other OpenAI models

**Fast Models for Quick Analysis:**
- **Ollama**: `qwen2.5-coder:7b`, `deepseek-coder-v2:16b` (local, free, very fast)
- **OpenRouter**: `google/gemini-2.0-flash-exp`, `anthropic/claude-haiku-4-5`, `deepseek/deepseek-chat`
- **Anthropic**: `claude-haiku-4-5` (fast Haiku 4.5, cost-effective)
- **Bedrock**: `anthropic.claude-haiku-4-5-v1:0` (fast Haiku 4.5 on AWS)
- **OpenAI**: `o3-mini` (fast reasoning model)

**Provider Configuration:**

Provider selection is configured in `.gavel/policies.yaml` via the `provider` section:

```yaml
# Ollama (local)
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b  # fast local model
    base_url: http://localhost:11434/v1

# OpenRouter (unified API)
provider:
  name: openrouter
  openrouter:
    model: google/gemini-2.0-flash-exp  # very fast

# Anthropic (direct API)
provider:
  name: anthropic
  anthropic:
    model: claude-haiku-4-5  # fast, cost-effective (recommended)

# AWS Bedrock
provider:
  name: bedrock
  bedrock:
    model: anthropic.claude-haiku-4-5-v1:0
    region: us-east-1

# OpenAI
provider:
  name: openai
  openai:
    model: gpt-5.3-codex  # or gpt-5.2 for general use
```

**Model Selection Guidance:**
- **Quality priority**: Anthropic Claude Opus 4.6 (Feb 2026) > Sonnet 4.5 > OpenAI GPT-5.3-Codex
- **Speed priority**: Ollama local models > Gemini Flash > Claude Haiku 4.5 > o3-mini
- **Cost priority**: Ollama (free) > DeepSeek > o3-mini > Claude Haiku 4.5 > GPT-5 > Claude Sonnet > Claude Opus

See `example-configs.yaml` for detailed provider examples with performance/cost comparisons.

The BAML client wrapper (`internal/analyzer/bamlclient.go`) dispatches to the appropriate generated client based on this config at runtime.

After changing `.baml` files, run `task generate`. The LLM provider is selected via config, not environment variables.

## Personas

Gavel uses BAML to implement switchable analysis personas. Different personas provide
specialized expert perspectives: code quality, architecture, or security.

**Implementation:**
- `internal/analyzer/personas.go` - Persona prompt constants and selection logic
- `internal/config/config.go` - Persona configuration field
- `docs/personas-feature-design.md` - Full design document

**To add a new persona:**
1. Add prompt constant to `internal/analyzer/personas.go`
2. Add case to `GetPersonaPrompt()` switch
3. Add to valid personas map in `internal/config/config.go` validation
4. Update documentation

**Current personas:**
- `code-reviewer` (default): Code quality, error handling, testability
- `architect`: Scalability, API design, service boundaries
- `security`: OWASP Top 10, auth/authz, injection vulnerabilities

## Rego

Default gate policy is in `internal/evaluator/default.rego`. Package `gavel.gate`, queried for `data.gavel.gate.decision`. Returns "reject" (error + confidence > 0.8), "merge" (no results), or "review" (default). Uses `import rego.v1` syntax (OPA v1.13.1).

## Release Process

Gavel uses Task-based builds with GitHub Actions for multi-platform releases.

**Creating a release:**
```bash
# Tag the release
task release VERSION=v0.2.0

# Push the tag
git push origin v0.2.0
```

**Build workflow:**
1. Linux and macOS runners build natively with CGO enabled
2. Each platform produces amd64 and arm64 binaries
3. Binaries are archived as `.tar.gz` with checksums
4. Final job creates GitHub release with all artifacts

**Local development build:**
```bash
task build              # Current platform only
task build:release      # All architectures for current OS
```

**Requirements:**
- CGO must be enabled (BAML dependency requires it)
- macOS builds require Xcode tools
- Linux builds use system GCC
