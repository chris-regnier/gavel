# Gavel

AI-powered code analysis CLI that gates CI workflows by analyzing code against configurable policies via an LLM, producing [SARIF](https://sarifweb.azurewebsites.net/) output, and evaluating it with [Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) to reach a verdict: **merge**, **reject**, or **review**.

## How It Works

```
Source Code → BAML Analyzer → SARIF Assembler → Rego Evaluator → Verdict
  (files,       (LLM finds      (standard         (policy-based      (merge,
   diffs,        violations)      format +           gating)           reject,
   dirs)                          extensions)                          review)
```

1. **Read** source artifacts — individual files, a unified diff, or a directory tree
2. **Analyze** each artifact against enabled policies using an LLM (via [BAML](https://docs.boundaryml.com/))
3. **Assemble** findings into a SARIF 2.1.0 log with gavel-specific extensions (confidence, explanation, recommendation)
4. **Evaluate** the SARIF log against Rego policies to produce a gating decision
5. **Store** both the SARIF log and verdict to `.gavel/results/`

## Installation

### Download Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/chris-regnier/gavel/releases).

```bash
# macOS (arm64)
curl -L https://github.com/chris-regnier/gavel/releases/latest/download/gavel_<version>_Darwin_arm64.tar.gz | tar xz
sudo mv gavel /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/chris-regnier/gavel/releases/latest/download/gavel_<version>_Linux_x86_64.tar.gz | tar xz
sudo mv gavel /usr/local/bin/

# Windows (amd64)
# Download the .zip file from the releases page and extract
```

### Build from Source

#### Prerequisites

- Go 1.25+
- [Task](https://taskfile.dev/) (task runner)
- [BAML CLI](https://docs.boundaryml.com/) (for regenerating the LLM client)
- An [OpenRouter](https://openrouter.ai/) API key

```bash
task build
```

This produces a `gavel` binary in the project root.

## Usage

```bash
export OPENROUTER_API_KEY=your-key-here

# Analyze a directory
./gavel analyze --dir ./src

# Analyze specific files
./gavel analyze --files main.go,handler.go

# Analyze a diff (e.g., from a PR)
git diff main...HEAD | ./gavel analyze --diff -

# Analyze a diff file
./gavel analyze --diff changes.patch
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--dir` | Directory to recursively scan | — |
| `--files` | Comma-separated list of files | — |
| `--diff` | Path to unified diff (`-` for stdin) | — |
| `--output` | Output directory for results | `.gavel/results` |
| `--policies` | Policy config directory | `.gavel` |
| `--rego` | Rego policies directory | `.gavel/rego` |

### Output

Gavel writes two files per run to `.gavel/results/<id>/`:

- **`sarif.json`** — Full SARIF 2.1.0 analysis results
- **`verdict.json`** — Gating decision with reasoning

```json
{
  "decision": "review",
  "reason": "Decision: review based on 3 findings",
  "relevant_findings": [
    {
      "ruleId": "shall-be-merged",
      "level": "error",
      "message": { "text": "Error from cmd.Execute() is silently discarded" },
      "locations": [{ "physicalLocation": { "artifactLocation": { "uri": "main.go" }, "region": { "startLine": 10, "endLine": 12 } } }],
      "properties": {
        "gavel/confidence": 0.9,
        "gavel/explanation": "The main function catches the error from Execute but discards it...",
        "gavel/recommendation": "Log the error and exit with a non-zero status code"
      }
    }
  ]
}
```

**Decisions:**

| Decision | Meaning | Trigger |
|----------|---------|---------|
| `merge` | Safe to auto-merge | No findings |
| `reject` | Block the merge | Any error-level finding with confidence > 0.8 |
| `review` | Needs human review | All other cases (default) |

### Using Ollama (Local LLMs)

Gavel supports local LLM analysis via [Ollama](https://ollama.ai/):

#### 1. Install and start Ollama

```bash
# macOS
brew install ollama

# Start Ollama server
ollama serve
```

#### 2. Pull a model

```bash
ollama pull gpt-oss:20b
```

#### 3. Configure Gavel

Create or edit `.gavel/policies.yaml`:

```yaml
provider:
  name: ollama
  ollama:
    model: gpt-oss:20b
    base_url: http://localhost:11434/v1  # optional, this is the default

policies:
  shall-be-merged:
    enabled: true
    severity: error
```

#### 4. Run analysis

```bash
./gavel analyze --dir ./src
```

#### Switching between providers

**To use OpenRouter instead:**

```yaml
provider:
  name: openrouter
  openrouter:
    model: anthropic/claude-sonnet-4
```

Then set your API key:

```bash
export OPENROUTER_API_KEY=your-key-here
./gavel analyze --dir ./src
```

## Personas

Gavel supports different analysis personas for specialized code review:

- `code-reviewer` (default): Focuses on code quality, bugs, and best practices
- `architect`: Focuses on system design, scalability, and API patterns
- `security`: Focuses on vulnerabilities and OWASP Top 10

### Using Personas

**Via config** (`.gavel/policies.yaml`):
```yaml
persona: security
```

**Via CLI flag**:
```bash
gavel analyze --persona architect --dir ./src
```

Different personas provide specialized expertise:
- Use `code-reviewer` for daily PR reviews
- Use `architect` for architecture reviews
- Use `security` for security audits before releases

## Configuration

Gavel uses a tiered policy configuration system. Policies are merged in order of precedence (highest wins):

1. **Project** — `.gavel/policies.yaml`
2. **Machine** — `~/.config/gavel/policies.yaml`
3. **System defaults** — built into the binary

### Policy Format

```yaml
policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error          # error, warning, or note
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true

  function-length:
    description: "Functions should not exceed a reasonable length"
    severity: note
    instruction: "Flag functions longer than 50 lines."
    enabled: true            # override the default (disabled)

  my-custom-policy:
    description: "No hardcoded secrets"
    severity: error
    instruction: "Flag any hardcoded API keys, passwords, or tokens."
    enabled: true
```

### Default Policies

| Policy | Severity | Default | Description |
|--------|----------|---------|-------------|
| `shall-be-merged` | error | enabled | Catch-all quality gate — flags risky, sloppy, untested, or overly complex code |
| `function-length` | note | disabled | Flags functions longer than 50 lines |

### Merging Rules

- Non-empty string fields from a higher tier override lower tier values
- Setting `enabled: true` in a higher tier enables a policy
- Setting _only_ `enabled: false` (with no other fields) disables a policy from a lower tier

## Custom Rego Policies

The default gate policy maps findings to decisions based on severity and confidence. To customize the gating logic, place `.rego` files in `.gavel/rego/`:

```rego
package gavel.gate

import rego.v1

default decision := "review"

# Reject on any error with high confidence
decision := "reject" if {
    some result in input.runs[0].results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.8
}

# Auto-merge if clean
decision := "merge" if {
    count(input.runs[0].results) == 0
}
```

The Rego policy receives the full SARIF log as `input`. It never sees source code directly — only the structured findings.

Custom `.rego` files in the rego directory override the embedded default policy entirely.

## SARIF Extensions

Gavel extends standard SARIF results with properties under the `gavel/` namespace:

| Property | Type | Description |
|----------|------|-------------|
| `gavel/confidence` | float (0.0–1.0) | LLM confidence in the finding |
| `gavel/explanation` | string | Detailed reasoning behind the finding |
| `gavel/recommendation` | string | Suggested fix or action |
| `gavel/inputScope` | string | Input type: `files`, `diff`, or `directory` |

## Architecture

```
cmd/gavel/           CLI entry point (Cobra)
internal/
  input/             Reads files, diffs, directories into artifacts
  config/            Tiered YAML policy configuration
  analyzer/          Orchestrates LLM analysis via BAML client
  sarif/             SARIF 2.1.0 assembly and deduplication
  evaluator/         Rego policy evaluation (OPA)
  store/             Filesystem persistence for results
baml_src/            BAML prompt templates (source of truth)
baml_client/         Generated Go client (do not edit)
```

## Development

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

### Releasing

Releases are automated via GitHub Actions and [GoReleaser](https://goreleaser.com/). To create a new release:

```bash
# Tag a new version (following semver)
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0

# GitHub Actions will automatically:
# 1. Run tests and linter
# 2. Generate BAML client
# 3. Build binaries for multiple platforms
# 4. Create a GitHub release with changelog
# 5. Upload release artifacts
```

To test the release process locally without publishing:

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser/v2@latest

# Run a local snapshot build
goreleaser release --snapshot --clean

# Check the dist/ directory for built artifacts
ls -la dist/
```

### BAML

LLM prompt templates live in `baml_src/`. After editing `.baml` files, run `task generate` to regenerate the Go client in `baml_client/`. The generated code should not be edited by hand.

The default LLM provider is Ollama with `gpt-oss:20b` running locally.

## License

[MIT](LICENSE)
