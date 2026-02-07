# Gavel Code Style and Conventions

## General Go Style
- Follow standard Go conventions (gofmt, go vet)
- Use meaningful variable names
- Keep functions focused and reasonably sized

## Architecture Patterns
- **Interface-based design**: Key abstractions like `Store` and `BAMLClient` are interfaces
- **Generated code isolation**: BAML-generated code in `baml_client/` should never be manually edited
- **Tiered configuration**: Config merging with clear precedence rules
- **Property namespacing**: Custom SARIF properties use `gavel/` prefix

## Key Design Decisions
- **BAMLClient interface** (`internal/analyzer/analyzer.go`): All tests use mock clients. `BAMLLiveClient` wraps generated code.
- **Type conversions**: Generated BAML types use `int64`/`RuleId`; internal types use `int`/`RuleID`
- **Config merging** (`internal/config/config.go`): Non-zero string fields override; `Enabled` bool always applies
- **SARIF extensions**: All gavel-specific data in `Properties map[string]interface{}` with `gavel/` keys
- **Rego isolation**: Rego policies receive only SARIF JSON, never raw source code
- **Storage IDs**: Format is `<timestamp>-<hex>` under `.gavel/results/`

## File Organization
- CLI commands in `cmd/gavel/`
- Internal packages in `internal/` (not importable by external projects)
- BAML templates in `baml_src/` (source of truth for LLM prompts)
- Generated BAML client in `baml_client/` (never edit manually)

## Testing
- Mock interfaces for LLM calls (no real API calls in unit tests)
- Integration test in `integration_test.go` at project root
- Test data and fixtures should be self-contained
