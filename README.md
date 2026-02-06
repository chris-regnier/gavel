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
- An LLM provider (see [Supported Providers](#supported-providers) below)

```bash
task build
```

This produces a `gavel` binary in the project root.

## Quick Start

```bash
# 1. Set up a provider (example: Ollama for local/free usage)
ollama pull qwen2.5-coder:7b

# 2. Configure Gavel (creates .gavel/policies.yaml)
cat > .gavel/policies.yaml <<EOF
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1
EOF

# 3. Analyze your code
./gavel analyze --dir ./src
```

## Usage

```bash
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

## Supported Providers

Gavel supports multiple LLM providers to fit different needs:

| Provider | Type | Cost | Speed | Best For |
|----------|------|------|-------|----------|
| **Ollama** | Local | Free | ⚡⚡⚡ Fast | Local development, privacy-sensitive work |
| **OpenRouter** | Cloud API | Pay-per-use | ⚡⚡ Variable | Easy access to many models |
| **Anthropic** | Cloud API | Premium | ⚡⚡ Fast | Production workloads, highest quality |
| **AWS Bedrock** | AWS Cloud | Premium | ⚡⚡ Fast | Enterprise AWS environments |
| **OpenAI** | Cloud API | Moderate | ⚡⚡⚡ Fast | General purpose, GPT-4 users |

### Ollama (Local, Free)

Perfect for local development and privacy-sensitive work.

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a fast code model
ollama pull qwen2.5-coder:7b

# Configure in .gavel/policies.yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1

# Run analysis
./gavel analyze --dir ./src
```

**Fast Ollama models:** `qwen2.5-coder:7b`, `deepseek-coder-v2:16b`, `codestral:22b`

### OpenRouter (Cloud, Pay-per-use)

Access multiple models through a unified API.

```bash
# Get API key from https://openrouter.ai/keys
export OPENROUTER_API_KEY=sk-or-...

# Configure in .gavel/policies.yaml
provider:
  name: openrouter
  openrouter:
    model: google/gemini-2.0-flash-001  # very fast

# Run analysis
./gavel analyze --dir ./src
```

**Recommended models:**
- `google/gemini-2.0-flash-exp` - Very fast, excellent value
- `anthropic/claude-haiku-4-5` - Fast Claude Haiku 4.5, good quality
- `deepseek/deepseek-chat` - Very cheap, surprisingly good
- `anthropic/claude-3-5-sonnet-20241022` - High quality Sonnet

### Anthropic (Direct API)

Direct access to Claude models for production workloads.

```bash
# Get API key from https://console.anthropic.com/
export ANTHROPIC_API_KEY=sk-ant-...

# Configure in .gavel/policies.yaml
provider:
  name: anthropic
  anthropic:
    model: claude-haiku-4-5

# Run analysis
./gavel analyze --dir ./src
```

**Available models:**
- `claude-haiku-4-5` - Fast, cost-effective (recommended)
- `claude-3-5-sonnet-20241022` - High quality, balanced
- `claude-opus-4-6-20260205` - Highest quality, released Feb 5, 2026

### AWS Bedrock (Enterprise)

Claude models on AWS infrastructure for enterprise deployments.

```bash
# Configure AWS credentials
aws configure

# Configure in .gavel/policies.yaml
provider:
  name: bedrock
  bedrock:
    model: anthropic.claude-haiku-4-5-v1:0
    region: us-east-1

# Run analysis
./gavel analyze --dir ./src
```

**Available models:**
- `anthropic.claude-haiku-4-5-v1:0` - Fast Haiku 4.5 (recommended)
- `global.anthropic.claude-sonnet-4-5-20250929-v1:0` - Sonnet 4.5 (global endpoint)
- `anthropic.claude-opus-4-6-20260205-v1:0` - Opus 4.6 (highest quality, released Feb 5, 2026)

### OpenAI (Cloud API)

GPT models for general-purpose code analysis.

```bash
# Get API key from https://platform.openai.com/api-keys
export OPENAI_API_KEY=sk-proj-...

# Configure in .gavel/policies.yaml
provider:
  name: openai
  openai:
    model: gpt-5.2

# Run analysis
./gavel analyze --dir ./src
```

**Recommended models:**
- `gpt-5.3-codex` - Latest coding-specialized model (recommended for code analysis)
- `gpt-5.2` - Newest flagship general model
- `o3-mini` - Fast reasoning model for math/science/coding

### Provider Comparison & Selection

**For Speed:**
1. Ollama `qwen2.5-coder:7b` (local, 1-3 sec/file)
2. OpenRouter `google/gemini-2.0-flash-exp` (cloud, 2-5 sec/file)
3. Anthropic `claude-haiku-4-5` (cloud, 3-6 sec/file)

**For Quality:**
1. Anthropic Claude Opus 4.6 (released Feb 5, 2026)
2. Anthropic Claude Sonnet 4.5
3. OpenAI GPT-5.3-Codex (for code analysis)

**For Cost:**
1. Ollama (free, local)
2. OpenRouter DeepSeek (~$0.20 per 100 files)
3. Anthropic Claude Haiku 4.5 (~$2.40 per 100 files)

**Detailed provider documentation:** See [docs/PROVIDERS.md](docs/PROVIDERS.md) and [example-configs.yaml](example-configs.yaml)

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
# Provider configuration (required)
provider:
  name: ollama  # or: openrouter, anthropic, bedrock, openai
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1

# Analysis policies
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

BAML client definitions for all providers are in `baml_src/clients.baml`. The default provider is Ollama with `gpt-oss:20b` running locally.

## License

[MIT](LICENSE)
