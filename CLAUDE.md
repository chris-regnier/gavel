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
- **Cache metadata & cross-environment sharing**: SARIF results include `gavel/cache_key` (deterministic hash of file content + policies + model + BAML templates) and `gavel/analyzer` metadata (provider, model, policies used). Cache keys enable sharing results across CI and local environments when analysis inputs match. Cache invalidation only occurs when LLM inputs change (file content, policy instructions, model, BAML templates), NOT when Rego policies or severity levels change (those only affect verdict evaluation, not SARIF generation).

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
- **OpenRouter**: `google/gemini-2.0-flash-001`, `anthropic/claude-3.5-haiku`, `deepseek/deepseek-chat`
- **Anthropic**: `claude-3-5-haiku-20241022` (fast, lower cost than Sonnet)
- **Bedrock**: `anthropic.claude-3-5-haiku-*` (fast Haiku on AWS)
- **OpenAI**: `gpt-4o-mini` (fast, cost-effective)

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
    model: google/gemini-2.0-flash-001  # very fast

# Anthropic (direct API)
provider:
  name: anthropic
  anthropic:
    model: claude-sonnet-4-20250514  # or claude-3-5-haiku-20241022 for speed

# AWS Bedrock
provider:
  name: bedrock
  bedrock:
    model: anthropic.claude-sonnet-4-5-v2:0
    region: us-east-1

# OpenAI
provider:
  name: openai
  openai:
    model: gpt-4o  # or gpt-4o-mini for speed
```

**Model Selection Guidance:**
- **Quality priority**: Anthropic Claude Opus 4.5/4.6 > Sonnet 4/4.5 > OpenAI GPT-4o
- **Speed priority**: Ollama local models > Gemini Flash > Claude Haiku > GPT-4o-mini
- **Cost priority**: Ollama (free) > DeepSeek > GPT-4o-mini > Claude Haiku > GPT-4o > Claude Sonnet > Claude Opus

See `example-configs.yaml` for detailed provider examples with performance/cost comparisons.

The BAML client wrapper (`internal/analyzer/bamlclient.go`) dispatches to the appropriate generated client based on this config at runtime.

After changing `.baml` files, run `task generate`. The LLM provider is selected via config, not environment variables.

## Rego

Default gate policy is in `internal/evaluator/default.rego`. Package `gavel.gate`, queried for `data.gavel.gate.decision`. Returns "reject" (error + confidence > 0.8), "merge" (no results), or "review" (default). Uses `import rego.v1` syntax (OPA v1.13.1).
