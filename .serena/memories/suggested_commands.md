# Suggested Commands for Gavel Development

## Build & Development
```bash
task build          # Builds binary to ./gavel
task test           # Runs all tests with verbose output
task lint           # Runs go vet linter
task generate       # Regenerates BAML client from baml_src/
```

## Running Tests
```bash
# All tests
go test ./... -v

# Single test
go test ./internal/config/ -run TestMergeOverrides -v

# Integration test only
go test -run TestIntegration -v
```

## Running Gavel
```bash
# Set API key (if using OpenRouter)
export OPENROUTER_API_KEY=your-key

# Analyze directory
./gavel analyze --dir ./internal/input

# Analyze files
./gavel analyze --files main.go,handler.go

# Analyze diff
git diff main...HEAD | ./gavel analyze --diff -

# Analyze diff file
./gavel analyze --diff changes.patch
```

## System Utilities (macOS/Darwin)
Standard Unix commands work on Darwin:
- `ls`, `cd`, `pwd` - directory navigation
- `grep`, `find` - searching
- `cat`, `less`, `tail` - file viewing
- `git` - version control

Note: Darwin is BSD-based, so some GNU flags may differ from Linux.
