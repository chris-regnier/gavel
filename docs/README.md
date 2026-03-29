# Gavel

> AI-powered code analysis CLI that gates CI workflows by analyzing code against configurable policies via an LLM, producing [SARIF](https://sarifweb.azurewebsites.net/) output. A separate `judge` command evaluates SARIF with [Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) to reach a verdict: **merge**, **reject**, or **review**.

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

1. **Read** source artifacts — individual files, a unified diff, or a directory tree
2. **Analyze** each artifact against enabled policies using an LLM (via [BAML](https://docs.boundaryml.com/))
3. **Assemble** findings into a SARIF 2.1.0 log with gavel-specific extensions (confidence, explanation, recommendation)
4. **Store** the SARIF log to `.gavel/results/`
5. **Judge** (separate command) evaluates the SARIF log against Rego policies to produce a gating decision and verdict

## Decisions

| Decision | Meaning | Trigger |
|----------|---------|---------|
| `merge` | Safe to auto-merge | No findings |
| `reject` | Block the merge | Any error-level finding with confidence > 0.8 |
| `review` | Needs human review | All other cases (default) |

## Next Steps

- **[Quick Start](quickstart.md)** — Get up and running in minutes
- **[Installation](installation.md)** — Download binaries or build from source
- **[Try on Open Source](guides/try-on-open-source.md)** — Run Gavel on real repositories
- **[Gate PRs with CI](guides/ci-pr-gating.md)** — Automate code review on every PR
- **[Editor Integration](guides/editor-integration.md)** — See findings inline in VS Code or Neovim
- **[Generating Configuration](guides/generating-config.md)** — Create policies, rules, and personas with AI
