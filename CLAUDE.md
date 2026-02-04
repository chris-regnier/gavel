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

Source templates live in `baml_src/`. Generated Go client is in `baml_client/` (do not edit). After changing `.baml` files, run `task generate`. The LLM provider is OpenRouter (`OPENROUTER_API_KEY` env var), model `anthropic/claude-sonnet-4`.

## Rego

Default gate policy is in `internal/evaluator/default.rego`. Package `gavel.gate`, queried for `data.gavel.gate.decision`. Returns "reject" (error + confidence > 0.8), "merge" (no results), or "review" (default). Uses `import rego.v1` syntax (OPA v1.13.1).
