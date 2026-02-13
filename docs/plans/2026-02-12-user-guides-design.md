# User Guides Design

**Date**: 2026-02-12
**Status**: Approved

## Overview

Create two task-oriented user guides that walk developers through specific end-to-end workflows with Gavel. These complement the README (which serves as a reference) by providing focused, copy-pasteable tutorials.

## Guides

### Guide 1: CI/PR Gating (`docs/guides/ci-pr-gating.md`)

**Title**: "Gate PRs with AI Code Review"

**Audience**: Developers and team leads who want to add automated AI code review to their GitHub Actions pipeline.

**End state**: Every PR to the repo is analyzed by Gavel. Findings appear as native GitHub Code Scanning annotations on the PR diff. PRs with critical issues get flagged; clean PRs pass automatically.

**Content outline**:

1. **What you'll build** — One paragraph summary of the end state
2. **Prerequisites** — Gavel binary, GitHub repo, LLM provider API key
3. **Step 1: Choose your provider** — Brief comparison table (speed/cost/quality), focus on OpenRouter and Anthropic as easiest cloud options for CI. Show how to add the API key as a GitHub Actions secret.
4. **Step 2: Add project config** — Create `.gavel/policies.yaml` with provider + default policies. Commit to repo.
5. **Step 3: Add the GitHub Actions workflow** — Complete `.github/workflows/gavel.yml` that:
   - Triggers on `pull_request`
   - Downloads Gavel binary from releases
   - Runs `gavel analyze --diff` on the PR diff
   - Uploads SARIF to GitHub Code Scanning via `github/codeql-action/upload-sarif@v3`
   - Optionally posts verdict as a PR comment
6. **Step 4: Test it** — Open a PR with a known issue, watch it run, see results
7. **Step 5: Customize** — Tune policies, add custom rules, change personas, adjust Rego thresholds. Brief examples, link to README for full reference.
8. **For teams** — Shared config in repo, machine-level config for org-wide defaults, provider cost considerations at scale
9. **Troubleshooting** — Common issues (API key not set, model unavailable, timeout on large diffs)
10. **Next steps** — Link to editor integration guide, README

### Guide 2: Editor Integration (`docs/guides/editor-integration.md`)

**Title**: "See Gavel Findings in Your Editor"

**Audience**: Developers who want to see AI-powered findings inline while coding, like a built-in linter.

**End state**: SARIF results displayed as diagnostics in VS Code (primary) or neovim (secondary), with inline annotations, hover details, and recommendations.

**Content outline**:

1. **What you'll build** — One paragraph summary
2. **Prerequisites** — Gavel binary, LLM provider configured, VS Code or neovim
3. **Step 1: Configure Gavel locally** — Create `.gavel/policies.yaml`. Recommend Ollama for local dev (free, fast, no API key). Brief cloud provider alternative.
4. **Step 2: Run an analysis** — `gavel analyze --dir ./src`. Show sample output, point to SARIF file location.
5. **Step 3a: VS Code** —
   - Install MS SARIF Viewer extension (`MS-SarifVSCode.sarif-viewer`)
   - Open SARIF file — findings appear in Problems panel and inline
   - Describe what the UX looks like (inline annotations, hover with explanation/recommendation)
   - Optional: VS Code task in `.vscode/tasks.json` for keyboard shortcut
6. **Step 3b: Neovim** —
   - Option A: `jq` one-liner to convert SARIF to quickfix format, then `:cfile`
   - Option B: Use `gavel review` TUI alongside neovim
   - Note that LSP integration is planned
7. **Step 4: Iterate** — Edit, re-run, reload SARIF. Show the workflow loop. Mention `review` TUI.
8. **For teams** — Shared `.gavel/policies.yaml` means consistent rules. CI SARIF artifacts can be downloaded and viewed locally.
9. **Tips** — Personas for focus, Ollama for speed, combine with `gavel review` TUI
10. **Next steps** — Link to CI guide, README, custom rules reference

## Design Decisions

### Approach: Task-Oriented Walkthroughs

Each guide walks through a specific workflow as a tutorial rather than serving as a reference (the README already does that). Users start with "what you'll achieve" and follow step-by-step to a working setup.

### SARIF as the integration layer

Rather than building custom editor plugins, we lean on the SARIF standard. GitHub Code Scanning natively accepts SARIF uploads, VS Code has a first-party SARIF viewer, and neovim can consume structured data via quickfix. This gives us broad editor support with zero custom tooling.

### GitHub Code Scanning for CI

Using `github/codeql-action/upload-sarif@v3` to upload Gavel's SARIF output to GitHub Code Scanning. This gives native PR diff annotations for free — no custom bot or comment logic needed.

### Self-contained but cross-linked

Each guide can be followed independently. Cross-references appear in "Next steps" sections. Both link to the README for full configuration reference rather than duplicating provider/policy documentation.

## Conventions

- Second person ("you"), imperative instructions
- Code blocks are complete and copy-pasteable
- Prose is short between code blocks — guides are skimmable
- "For teams" callout sections address the organizational audience without splitting into separate paths
- Provider examples link to `example-configs.yaml` and README to minimize maintenance surface

## File Locations

```
docs/
  guides/
    ci-pr-gating.md
    editor-integration.md
```
