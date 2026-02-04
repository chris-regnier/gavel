# Gavel Code Style and Conventions

## Go Style
- Standard Go conventions apply
- Use `go vet` for linting
- Package names match directory names
- Interfaces defined in the package that uses them

## BAML Conventions
- Source of truth: `baml_src/*.baml`
- Generated code in `baml_client/` - NEVER edit directly
- Client definitions in `clients.baml`
- Function definitions in `analyze.baml`
- Retry policies defined alongside clients

## Configuration
- Tiered config: system defaults → `~/.config/gavel/policies.yaml` → `.gavel/policies.yaml`
- YAML format for policies
- Non-empty string fields override lower tiers
- `Enabled` bool always applies

## SARIF Extensions
- All gavel-specific data uses `gavel/` prefix in properties
- Standard properties: `gavel/confidence`, `gavel/explanation`, `gavel/recommendation`

## Naming Conventions
- Internal types use `Finding` with `RuleID` (string)
- Generated BAML types use `Finding` with `RuleId` (string)
- Type conversion happens in `bamlclient.go`

## Error Handling
- Return errors, don't panic
- Use `fmt.Errorf` for wrapping errors
- Test error paths explicitly

## Testing
- Interface-based design enables mocking
- `BAMLClient` interface has mock implementation for tests
- Integration tests use actual BAML client
