# Contributing

## Development Commands

```bash
task build           # Build the binary
task test            # Run all tests
task lint            # Run go vet
task generate        # Regenerate BAML client from baml_src/

# Run a single test
go test ./internal/config/ -run TestMergeOverrides -v

# Run the integration test
go test -run TestIntegration -v
```

## BAML

LLM prompt templates live in `baml_src/`. After editing `.baml` files, run `task generate` to regenerate the Go client in `baml_client/`. The generated code should not be edited by hand.

## Releasing

Releases are automated via GitHub Actions and [Task](https://taskfile.dev/):

```bash
# Recommended: use the release task (validates, tests, tags, and pushes)
task release VERSION=v0.2.0

# Or manually:
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

GitHub Actions will automatically:
1. Run tests
2. Build binaries for Linux, macOS (amd64 + arm64)
3. Create a GitHub release with all artifacts

### Local Build Testing

```bash
# Build for current platform
task build

# Build release binaries for current OS (amd64 + arm64)
task build:release

# Check the dist/ directory for built artifacts
ls -la dist/
```
