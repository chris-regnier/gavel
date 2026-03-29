# Gavel

AI-powered code analysis CLI that gates CI workflows by analyzing code against configurable policies via an LLM, producing [SARIF](https://sarifweb.azurewebsites.net/) output. A separate `judge` command evaluates SARIF with [Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) to reach a verdict: **merge**, **reject**, or **review**.

**[Documentation](https://chris-regnier.github.io/gavel/)**

## How It Works

```
analyze: Source Code → BAML Analyzer → SARIF Assembler → FileStore
           (files,       (LLM finds      (standard          (SARIF
            diffs,        violations)      format +           saved)
            dirs)                          extensions)

judge:   FileStore → Rego Evaluator → Verdict
          (reads        (policy-based      (merge,
           SARIF)        gating)            reject,
                                            review)
```

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

# 4. Judge the results
./gavel judge
```

## Installation

Download the latest release for your platform from the [releases page](https://github.com/chris-regnier/gavel/releases), or build from source:

```bash
task build
```

See the [installation docs](https://chris-regnier.github.io/gavel/#/installation) for detailed instructions.

## Documentation

Full documentation is available at **[chris-regnier.github.io/gavel](https://chris-regnier.github.io/gavel/)**.

- **[Quick Start](https://chris-regnier.github.io/gavel/#/quickstart)** — Get up and running in minutes
- **[Providers](https://chris-regnier.github.io/gavel/#/PROVIDERS)** — Configure Ollama, OpenRouter, Anthropic, Bedrock, or OpenAI
- **[Policies & Rules](https://chris-regnier.github.io/gavel/#/configuration/policies)** — Customize what Gavel checks for
- **[Personas](https://chris-regnier.github.io/gavel/#/configuration/personas)** — Switch between code review, architecture, and security
- **[CLI Reference](https://chris-regnier.github.io/gavel/#/reference/cli)** — All commands and flags

### Guides

- **[Try on Open Source](https://chris-regnier.github.io/gavel/#/guides/try-on-open-source)** — Run Gavel on real repositories in Go, Python, and TypeScript
- **[Gate PRs with CI](https://chris-regnier.github.io/gavel/#/guides/ci-pr-gating)** — Set up GitHub Actions to automatically analyze every PR
- **[Editor Integration](https://chris-regnier.github.io/gavel/#/guides/editor-integration)** — View findings inline in VS Code or Neovim

## Development

```bash
task build           # Build the binary
task test            # Run all tests
task lint            # Run go vet
task generate        # Regenerate BAML client from baml_src/
```

See the [contributing guide](https://chris-regnier.github.io/gavel/#/development/contributing) for more details.

## License

[MIT](LICENSE)
