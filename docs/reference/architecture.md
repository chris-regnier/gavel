# Architecture

## Project Structure

```
cmd/gavel/           CLI entry point (Cobra)
internal/
  input/             Reads files, diffs, directories into artifacts
  config/            Tiered YAML policy configuration
  rules/             Vendable rule packs (YAML schema, loader, embedded defaults)
  analyzer/          Orchestrates LLM analysis via BAML client
  astcheck/          Tree-sitter-based structural analysis
  sarif/             SARIF 2.1.0 assembly and deduplication
  evaluator/         Rego policy evaluation (OPA)
  store/             Filesystem persistence for results
baml_src/            BAML prompt templates (source of truth)
baml_client/         Generated Go client (do not edit)
```

## Pipeline

### `analyze`

1. Load tiered config (system defaults -> `~/.config/gavel/policies.yaml` -> `.gavel/policies.yaml`)
2. Load tiered rules (embedded defaults -> `~/.config/gavel/rules/*.yaml` -> `.gavel/rules/*.yaml`)
3. Read artifacts via input handler (files, unified diff, or directory walk)
4. Run instant-tier analysis (regex pattern matching + AST checks via tree-sitter)
5. Format enabled policies into text, call BAML `AnalyzeCode` per artifact
6. Convert findings to SARIF results with `gavel/` property extensions
7. Deduplicate overlapping findings, assemble SARIF 2.1.0 log
8. Store SARIF, output analysis summary JSON

### `judge`

1. Load tiered config
2. Resolve result ID (provided via `--result` or most recent from store)
3. Read SARIF from store
4. Evaluate with Rego, store verdict, output JSON

## Key Design Decisions

- **`BAMLClient` interface** — All tests use a mock client. `BAMLLiveClient` wraps the generated BAML client.
- **Tiered config merging** — Non-zero string fields override; `Enabled` bool always applies.
- **SARIF extensions** — All gavel-specific data lives in `Properties` with `gavel/` prefix keys.
- **Rego evaluator** — Default policy is embedded via `//go:embed`. Custom `.rego` files override it entirely.
- **Storage** — `Store` interface with filesystem implementation. IDs are `<timestamp>-<hex>` directories.
- **AST checks** — Tree-sitter-based structural analysis. `Check` interface with a `Registry` pattern.
- **Cache metadata** — SARIF results include `gavel/cache_key` for cross-environment sharing.
