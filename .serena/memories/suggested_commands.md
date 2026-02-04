# Suggested Commands for Gavel Development

## Build Commands
```bash
task build          # Build the gavel binary (go build -o gavel ./cmd/gavel)
task test           # Run all tests (go test ./... -v)
task lint           # Run linter (go vet ./...)
task generate       # Regenerate BAML client from baml_src/ (baml-cli generate)
```

## Running Gavel
```bash
# Set API key
export OPENROUTER_API_KEY=your-key-here

# Analyze a directory
./gavel analyze --dir ./internal/input

# Analyze specific files
./gavel analyze --files main.go,handler.go

# Analyze a diff
git diff main...HEAD | ./gavel analyze --diff -
```

## Test Commands
```bash
# Run all tests
go test ./... -v

# Run a specific test
go test ./internal/config/ -run TestMergeOverrides -v

# Run integration test only
go test -run TestIntegration -v
```

## BAML Workflow
1. Edit templates in `baml_src/*.baml`
2. Run `task generate` to regenerate Go client
3. Never edit `baml_client/` directly (auto-generated)

## System Commands (Darwin/macOS)
Standard Unix commands work: `git`, `ls`, `cd`, `grep`, `find`, etc.
