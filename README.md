# Gavel

AI code review that catches what linters miss — security holes, ignored errors, risky patterns — and gates your CI pipeline automatically.

## What You Get

Run Gavel on your code and get findings like this:

```json
{
  "decision": "reject",
  "reason": "Decision: reject based on 2 findings",
  "relevant_findings": [
    {
      "ruleId": "S3649",
      "level": "error",
      "message": {
        "text": "SQL injection: raw user input interpolated into query via fmt.Sprintf"
      },
      "properties": {
        "gavel/confidence": 0.95,
        "gavel/explanation": "The function builds a SQL query using fmt.Sprintf with user-supplied input directly interpolated, allowing SQL injection.",
        "gavel/recommendation": "Use parameterized queries with placeholder arguments instead of string interpolation."
      }
    }
  ]
}
```

Every finding includes a confidence score, an explanation of *why* it's a problem, and a concrete recommendation to fix it.

## Why Gavel

- **Catches real bugs.** Not just style nits. Security vulnerabilities, ignored errors, risky patterns, overly complex code. Gavel understands context — it knows when user input flows into a SQL query, not just that a query exists.
- **Gates CI automatically.** Every PR gets a verdict: merge, reject, or review. Findings appear as native GitHub annotations. High-confidence issues block the merge; everything else goes to human review.
- **Works with any LLM.** Run free and local with Ollama, or use OpenRouter, Anthropic, AWS Bedrock, or OpenAI. Switch models per environment — fast/cheap in CI, thorough for releases.

## Quick Start

```bash
# Install (see https://github.com/chris-regnier/gavel/releases for latest version)
VERSION=v0.2.0
curl -L "https://github.com/chris-regnier/gavel/releases/download/${VERSION}/gavel_${VERSION}_Darwin_arm64.tar.gz" | tar xz
sudo mv gavel_Darwin_arm64 /usr/local/bin/gavel

# Set up a provider (pick one)
export OPENROUTER_API_KEY=sk-or-...          # Cloud: fast, pay-per-use
# OR: ollama pull qwen2.5-coder:7b           # Local: free, private

# Generate a config from a description of your project
gavel create config "Go REST API with PostgreSQL"

# Analyze and judge
gavel analyze --dir ./src
gavel judge
```

See the **[full quickstart](https://chris-regnier.github.io/gavel/#/quickstart)** for detailed setup.

## What It Catches

| Category | Example Finding | Rule |
|----------|----------------|------|
| Security | SQL injection via string concatenation | S3649 |
| Security | Hardcoded API keys and passwords | S2068 |
| Security | OS command injection from user input | S2076 |
| Reliability | Error return value silently ignored | S1086 |
| Reliability | Empty error handler (`if err != nil {}`) | AST003 |
| Maintainability | Function exceeds 50 lines | AST001 |
| Maintainability | Nesting depth exceeds 4 levels | AST002 |

19 built-in rules (regex + tree-sitter AST) run instantly with no LLM call. The LLM finds deeper issues that pattern matching can't.

## How It Works

```
analyze: Source Code → LLM Analyzer → SARIF Output → Results Store
           (files,       (AI finds        (standard        (findings
            diffs,        real bugs)       format)          saved)
            dirs)

judge:   Results Store → Rego Evaluator → Verdict
           (reads           (policy-based     (merge,
            findings)        gating)           reject,
                                               review)
```

Gavel produces standard [SARIF 2.1.0](https://sarifweb.azurewebsites.net/) output that integrates with GitHub Code Scanning, VS Code, and any SARIF-compatible tool.

## Documentation

Full docs at **[chris-regnier.github.io/gavel](https://chris-regnier.github.io/gavel/)**.

- **[Quick Start](https://chris-regnier.github.io/gavel/#/quickstart)** — First finding in 5 minutes
- **[For Developers](https://chris-regnier.github.io/gavel/#/guides/for-developers)** — AI code review in your editor, before you push
- **[For CI/CD](https://chris-regnier.github.io/gavel/#/guides/for-ci-cd)** — Automated review on every pull request
- **[For Security Teams](https://chris-regnier.github.io/gavel/#/guides/for-security-teams)** — Catch OWASP Top 10 in every PR
- **[Providers](https://chris-regnier.github.io/gavel/#/PROVIDERS)** — Ollama, OpenRouter, Anthropic, Bedrock, OpenAI
- **[CLI Reference](https://chris-regnier.github.io/gavel/#/reference/cli)** — All commands and flags

### Guides

- **[Try on Open Source](https://chris-regnier.github.io/gavel/#/guides/try-on-open-source)** — Run Gavel on real Go, Python, and TypeScript repos
- **[Gate PRs with CI](https://chris-regnier.github.io/gavel/#/guides/ci-pr-gating)** — GitHub Actions workflow for every PR
- **[Editor Integration](https://chris-regnier.github.io/gavel/#/guides/editor-integration)** — Findings inline in VS Code or Neovim
- **[Generate Config with AI](https://chris-regnier.github.io/gavel/#/guides/generating-config)** — Create policies, rules, and personas from natural language

## Development

```bash
task build           # Build the binary
task test            # Run all tests
task lint            # Run go vet
task generate        # Regenerate BAML client from baml_src/
```

See the [contributing guide](https://chris-regnier.github.io/gavel/#/development/contributing).

## License

[MIT](LICENSE)
