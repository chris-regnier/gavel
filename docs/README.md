# Gavel

> AI code review that catches what linters miss — security holes, ignored errors, risky patterns — and gates your CI pipeline automatically.

## What You Get

Run Gavel on your code. Every finding includes a confidence score, an explanation of why it's a problem, and a concrete fix:

```json
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
```

Then judge the results to get a gating decision for your CI pipeline:

| Decision | Meaning | Trigger |
|----------|---------|---------|
| `merge` | Safe to auto-merge | No findings |
| `reject` | Block the merge | Any error-level finding with confidence > 0.8 |
| `review` | Needs human review | All other cases (default) |

## Why Gavel

- **Catches real bugs.** Security vulnerabilities, ignored errors, risky patterns, overly complex code. Understands context — not just pattern matching.
- **Gates CI automatically.** Findings appear as native GitHub annotations. High-confidence issues block the merge; the rest go to human review.
- **Works with any LLM.** Ollama (free, local), OpenRouter, Anthropic, AWS Bedrock, or OpenAI. Switch per environment.

## Get Started

- **[Quick Start](quickstart.md)** — First finding in 5 minutes
- **[Installation](installation.md)** — Download binaries or build from source

## Use Cases

- **[For Developers](guides/for-developers.md)** — AI code review in your editor, before you push
- **[For CI/CD](guides/for-ci-cd.md)** — Automated review on every pull request
- **[For Security Teams](guides/for-security-teams.md)** — Catch OWASP Top 10 in every PR

## Guides

- **[Try on Open Source](guides/try-on-open-source.md)** — Run Gavel on real Go, Python, and TypeScript repos
- **[Gate PRs with CI](guides/ci-pr-gating.md)** — GitHub Actions workflow for every PR
- **[Editor Integration](guides/editor-integration.md)** — See findings inline in VS Code or Neovim
- **[Generate Config with AI](guides/generating-config.md)** — Create policies, rules, and personas from natural language
