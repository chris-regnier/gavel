# Design: Try Gavel on Open Source Code Guide

**Date:** 2026-02-13
**Type:** Onboarding tutorial (docs/guides/ page)
**File:** `docs/guides/try-on-open-source.md`

## Purpose

Step-by-step tutorial for new users to try Gavel on real code immediately after installation. Uses three public, intentionally vulnerable repositories across Go, Python, and JavaScript to demonstrate all three input modes and guarantee interesting findings.

## Structure

1. Prerequisites & provider setup
2. Example 1: Go — directory scan (`--dir`)
3. Example 2: Python — file-targeted scan (`--files`)
4. Example 3: JavaScript — diff scan (`--diff`)
5. What's next (links to CI guide, editor integration, custom rules)

## Provider Setup

Three providers presented with copy-paste configs:

| Provider | Setup | Cost |
|----------|-------|------|
| Ollama | `ollama pull qwen2.5-coder:7b` | Free |
| OpenRouter | API key from openrouter.ai | ~$0.04/100 files |
| Anthropic | API key from console.anthropic.com | ~$2.40/100 files |

Each example includes `.gavel/policies.yaml` blocks for all three providers.

## Example 1: Go — `0c34/govwa`

- **Repo:** Go Vulnerable Web Application (~15 Go files)
- **Mode:** `gavel analyze --dir .`
- **Expected findings:** SQL injection (S3649), hardcoded credentials (S2068), error-ignored (S1086), empty error handlers (AST003), debug prints (S106)
- **Extras:** Re-run with `--persona security` to show persona switching

### Walkthrough

1. Clone repo
2. Create `.gavel/policies.yaml`
3. Run `gavel analyze --dir .`
4. Annotated verdict JSON walkthrough (decision, reason, relevant_findings)
5. Dig into `.gavel/results/` SARIF file (gavel/explanation, gavel/recommendation)
6. Re-run with `--persona security`

## Example 2: Python — `OWASP/PyGoat`

- **Repo:** Django-based intentionally vulnerable Python app
- **Mode:** `gavel analyze --files <2-3 specific files>`
- **Expected findings:** Hardcoded credentials (S2068), TODO/FIXME (S1135), long functions (AST001), deep nesting (AST002)

### Walkthrough

1. Clone repo
2. Create `.gavel/policies.yaml`
3. Run `gavel analyze --files <selected files>`
4. Annotated verdict JSON walkthrough
5. Dig into SARIF findings

## Example 3: JavaScript — `gothinkster/node-express-realworld-example-app`

- **Repo:** Express.js RealWorld API implementation (~20 files)
- **Mode:** `gavel analyze --diff`
- **Expected findings:** TODO comments (S1135), function-length (AST001), possibly hardcoded credentials in config

### Walkthrough

1. Clone repo
2. Create `.gavel/policies.yaml`
3. Generate diff from commit range: `git diff <older>..<newer> > changes.patch`
4. Run `gavel analyze --diff changes.patch`
5. Also show stdin variant: `git diff HEAD~3..HEAD | gavel analyze --diff -`
6. Annotated verdict JSON walkthrough

## Per-Example Template

Each example follows:

1. **Clone** — `git clone` + `cd`
2. **Configure** — `.gavel/policies.yaml` with all three provider options
3. **Run** — the `gavel analyze` command
4. **Read the output** — annotated verdict JSON
5. **Dig deeper** — SARIF file in `.gavel/results/`, `gavel/explanation` and `gavel/recommendation` properties

## Resilience

- Sample output blocks included so users know what to expect even if repos evolve
- Each example notes "your exact findings may differ"
- Examples are self-contained — user can jump to any one

## Style

- ~400-500 lines of markdown
- Direct, second-person tone ("Clone the repo", "Run the command")
- Consistent with existing guides (ci-pr-gating.md, editor-integration.md)
- Title: "Try Gavel on Open Source Code"

## What's Next Section

Links to:
- [Gate PRs with AI Code Review](ci-pr-gating.md)
- [See Findings in Your Editor](editor-integration.md)
- Custom rules and personas (README sections)
