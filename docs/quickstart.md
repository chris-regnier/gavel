# Quick Start

## 1. Set Up a Provider

The fastest way to get started is with [Ollama](https://ollama.ai/) for free, local analysis:

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a fast code model
ollama pull qwen2.5-coder:7b
```

Or use a cloud provider — see [Providers](PROVIDERS.md) for all options.

## 2. Configure Gavel

Create a `.gavel/policies.yaml` in your project root:

```yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1
```

## 3. Analyze Your Code

```bash
# Analyze a directory
gavel analyze --dir ./src

# Analyze specific files
gavel analyze --files main.go,handler.go

# Analyze a diff (e.g., from a PR)
git diff main...HEAD | gavel analyze --diff -
```

## 4. Judge the Results

```bash
gavel judge
```

The judge command evaluates findings against Rego policies and returns a verdict:

```json
{
  "decision": "review",
  "reason": "Decision: review based on 3 findings"
}
```

## What Next?

- **[Providers](PROVIDERS.md)** — Configure cloud providers for higher quality or faster analysis
- **[Policies & Rules](configuration/policies.md)** — Customize what Gavel checks for
- **[Personas](configuration/personas.md)** — Switch between code review, architecture, and security perspectives
- **[Gate PRs with CI](guides/ci-pr-gating.md)** — Automate analysis on every pull request
