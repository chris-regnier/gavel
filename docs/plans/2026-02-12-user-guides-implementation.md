# User Guides Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create two task-oriented user guides — CI/PR gating with GitHub Actions, and editor integration via SARIF viewers.

**Architecture:** Pure documentation. Two markdown files in `docs/guides/` plus a README update to link to them. No code changes.

**Tech Stack:** Markdown, GitHub Actions YAML, VS Code tasks JSON

---

### Task 1: Write the CI/PR Gating Guide

**Files:**
- Create: `docs/guides/ci-pr-gating.md`

**Step 1: Write the guide**

Create `docs/guides/ci-pr-gating.md` with the following complete content:

```markdown
# Gate PRs with AI Code Review

Set up Gavel to automatically analyze every pull request with an AI code reviewer. Findings appear as annotations directly on your PR diffs in GitHub, and PRs with critical issues get flagged.

## Prerequisites

- A GitHub repository
- An LLM provider API key (see [Supported Providers](../../README.md#supported-providers))

## Step 1: Choose Your Provider

Pick a provider based on your priorities:

| Priority | Recommended Provider | Model | Cost |
|----------|---------------------|-------|------|
| **Easy setup** | OpenRouter | `anthropic/claude-haiku-4-5` | ~$2.40 per 100 files |
| **Quality** | Anthropic | `claude-sonnet-4-5` | ~$3.00/M input tokens |
| **Budget** | OpenRouter | `deepseek/deepseek-chat` | ~$0.20 per 100 files |

Add your API key as a GitHub Actions secret:

1. Go to your repo → **Settings** → **Secrets and variables** → **Actions**
2. Click **New repository secret**
3. Name it `OPENROUTER_API_KEY` (or `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc.)
4. Paste your API key

## Step 2: Add Project Config

Create `.gavel/policies.yaml` in your repository root:

```yaml
provider:
  name: openrouter
  openrouter:
    model: anthropic/claude-haiku-4-5

policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true
```

Commit this file to your repo. Every contributor will share the same analysis config.

> **Using a different provider?** See [example-configs.yaml](../../example-configs.yaml) for Anthropic, Ollama, Bedrock, and OpenAI configurations.

## Step 3: Add the GitHub Actions Workflow

Create `.github/workflows/gavel.yml`:

```yaml
name: Gavel Code Review

on:
  pull_request:
    branches: [main]

permissions:
  contents: read
  security-events: write  # Required for SARIF upload

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Download Gavel
        run: |
          GAVEL_VERSION=$(curl -s https://api.github.com/repos/chris-regnier/gavel/releases/latest | jq -r .tag_name)
          curl -L "https://github.com/chris-regnier/gavel/releases/download/${GAVEL_VERSION}/gavel_${GAVEL_VERSION#v}_Linux_x86_64.tar.gz" | tar xz
          chmod +x gavel

      - name: Analyze PR diff
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
        run: |
          gh pr diff ${{ github.event.pull_request.number }} > pr.diff
          ./gavel analyze --diff pr.diff
        env:
          GH_TOKEN: ${{ github.token }}

      - name: Upload SARIF to GitHub Code Scanning
        if: always()
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: .gavel/results/
          category: gavel
```

This workflow:
1. Downloads the latest Gavel release
2. Gets the PR diff using the GitHub CLI
3. Runs Gavel analysis on the diff
4. Uploads the SARIF results to GitHub Code Scanning

GitHub Code Scanning renders findings as annotations directly on the PR diff — no custom bot or comment logic needed.

## Step 4: Test It

1. Commit both `.gavel/policies.yaml` and `.github/workflows/gavel.yml` to a branch
2. Open a PR with some code that has an obvious issue (e.g., an unhandled error, a hardcoded secret, or an overly complex function)
3. Watch the "Gavel Code Review" check run in the PR's Checks tab
4. Once complete, findings appear as annotations on the PR's **Files changed** tab

## Step 5: Customize

### Tune policies

Add or modify policies in `.gavel/policies.yaml`:

```yaml
policies:
  shall-be-merged:
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true

  function-length:
    severity: note
    instruction: "Flag functions longer than 50 lines."
    enabled: true

  no-hardcoded-secrets:
    description: "No hardcoded secrets"
    severity: error
    instruction: "Flag any hardcoded API keys, passwords, or tokens."
    enabled: true
```

### Use a specialized persona

Focus the analysis on security before a release:

```yaml
# In .github/workflows/gavel.yml, change the analyze step:
- name: Analyze PR diff
  run: |
    gh pr diff ${{ github.event.pull_request.number }} > pr.diff
    ./gavel analyze --diff pr.diff --persona security
```

Available personas: `code-reviewer` (default), `architect`, `security`.

### Add custom rules

Create `.gavel/rules/custom.yaml` with pattern-based rules. See [Custom Rules](../../README.md#custom-rules) in the README for the full rule format.

### Adjust the gate threshold

Create `.gavel/rego/gate.rego` to change when PRs get blocked:

```rego
package gavel.gate

import rego.v1

default decision := "review"

# Only reject on very high confidence errors
decision := "reject" if {
    some result in input.runs[0].results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.9
}

decision := "merge" if {
    count(input.runs[0].results) == 0
}
```

## For Teams

- **Shared config**: Commit `.gavel/policies.yaml` to the repo so every PR gets the same analysis
- **Org-wide defaults**: Place a `policies.yaml` at `~/.config/gavel/policies.yaml` on your CI runner (or bake it into a custom runner image) to set organization-wide defaults that projects can override
- **Cost at scale**: For high-volume repos, consider using `deepseek/deepseek-chat` via OpenRouter (~$0.20 per 100 files) or running Ollama on a self-hosted runner (free)

## Troubleshooting

| Issue | Solution |
|-------|----------|
| "API key not set" error | Verify the secret name in GitHub matches the env var in the workflow (e.g., `OPENROUTER_API_KEY`) |
| Timeout on large diffs | Split large PRs into smaller ones, or increase the workflow timeout with `timeout-minutes: 30` |
| "Model not found" error | Check the model name matches your provider's format (see [example-configs.yaml](../../example-configs.yaml)) |
| No annotations on PR | Ensure the repo has GitHub Code Scanning enabled (Settings → Code security → Code scanning) |
| SARIF upload fails | Verify the `security-events: write` permission is set in the workflow |

## Next Steps

- **See findings in your editor**: [Editor Integration Guide](editor-integration.md)
- **Full configuration reference**: [README](../../README.md#configuration)
- **Custom rules**: [README - Custom Rules](../../README.md#custom-rules)
```

**Step 2: Review the guide for accuracy**

Verify:
- The GHA workflow YAML is valid
- The `upload-sarif` action path `.gavel/results/` matches Gavel's output directory
- Provider secret names match what Gavel expects
- Links to README sections are correct

**Step 3: Commit**

```bash
git add docs/guides/ci-pr-gating.md
git commit -m "docs: add CI/PR gating user guide"
```

---

### Task 2: Write the Editor Integration Guide

**Files:**
- Create: `docs/guides/editor-integration.md`

**Step 1: Write the guide**

Create `docs/guides/editor-integration.md` with the following complete content:

```markdown
# See Gavel Findings in Your Editor

Run Gavel on your code locally and see AI-powered findings as inline diagnostics in your editor — just like a built-in linter. This guide covers VS Code and Neovim.

## Prerequisites

- [Gavel](../../README.md#installation) installed
- An LLM provider configured (see below)
- VS Code or Neovim

## Step 1: Configure Gavel Locally

Create `.gavel/policies.yaml` in your project root. For local development, Ollama is recommended (free, fast, no API key):

```yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1

policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true
```

If you don't have Ollama installed:

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull the model
ollama pull qwen2.5-coder:7b
```

> **Prefer a cloud provider?** Replace the `provider` section with any provider from [example-configs.yaml](../../example-configs.yaml). You'll need the corresponding API key set as an environment variable.

## Step 2: Run an Analysis

Analyze your project:

```bash
# Analyze a directory
gavel analyze --dir ./src

# Or analyze specific files
gavel analyze --files main.go,handler.go
```

Gavel prints the verdict to stdout and writes two files:
- `.gavel/results/<id>/sarif.json` — SARIF 2.1.0 analysis results
- `.gavel/results/<id>/verdict.json` — Gating decision

The SARIF file is what your editor will consume.

## Step 3a: VS Code

### Install the SARIF Viewer Extension

1. Open VS Code
2. Go to Extensions (Ctrl+Shift+X / Cmd+Shift+X)
3. Search for **"SARIF Viewer"** by Microsoft (`MS-SarifVSCode.sarif-viewer`)
4. Click Install

### View Findings

1. Run `gavel analyze --dir .` in your project
2. In VS Code, open the Command Palette (Ctrl+Shift+P / Cmd+Shift+P)
3. Type "SARIF: Open SARIF Log" and select it
4. Navigate to `.gavel/results/` and open the latest `sarif.json`

Findings appear in three places:
- **Problems panel** (Ctrl+Shift+M / Cmd+Shift+M) — all findings listed with severity
- **Inline in the editor** — squiggly underlines on affected lines
- **Hover tooltip** — hover over a finding to see the explanation and recommendation

### Optional: Add a VS Code Task

Add a keyboard shortcut to run Gavel without leaving the editor. Create `.vscode/tasks.json`:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Gavel: Analyze Project",
      "type": "shell",
      "command": "gavel analyze --dir . && echo 'Done. Open SARIF: Cmd+Shift+P → SARIF: Open SARIF Log'",
      "group": "test",
      "presentation": {
        "echo": true,
        "reveal": "always",
        "panel": "shared"
      },
      "problemMatcher": []
    }
  ]
}
```

Run it with **Terminal → Run Task → Gavel: Analyze Project**, or bind it to a keyboard shortcut in your keybindings.

## Step 3b: Neovim

### Option A: SARIF to Quickfix

Convert Gavel's SARIF output to Neovim's quickfix format with `jq`, then load it:

```bash
# Find the latest SARIF file and convert to quickfix format
LATEST=$(ls -td .gavel/results/*/ | head -1)
jq -r '.runs[0].results[] |
  .locations[0].physicalLocation as $loc |
  "\($loc.artifactLocation.uri):\($loc.region.startLine):1: \(.level): \(.message.text)"
' "${LATEST}sarif.json" > /tmp/gavel-qf.txt
```

In Neovim:
```vim
:cfile /tmp/gavel-qf.txt
:copen
```

Navigate findings with `:cnext` and `:cprev`.

You can wrap this into a shell alias for convenience:

```bash
# Add to ~/.bashrc or ~/.zshrc
gavel-qf() {
  gavel analyze --dir "${1:-.}"
  LATEST=$(ls -td .gavel/results/*/ | head -1)
  jq -r '.runs[0].results[] |
    .locations[0].physicalLocation as $loc |
    "\($loc.artifactLocation.uri):\($loc.region.startLine):1: \(.level): \(.message.text)"
  ' "${LATEST}sarif.json" > /tmp/gavel-qf.txt
  echo "Load in nvim with :cfile /tmp/gavel-qf.txt"
}
```

### Option B: Use the Review TUI

Gavel includes a built-in terminal UI for reviewing findings. Run it alongside Neovim in a split terminal:

```bash
gavel review --dir ./src
```

The TUI shows a file tree, inline code view, and finding details. Navigate with arrow keys, accept or reject findings, and switch between files.

> **Coming soon:** Gavel LSP mode will provide native in-editor diagnostics without manual SARIF loading. See the [LSP integration design](../plans/2026-02-05-lsp-integration-design.md) for details.

## Step 4: Iterate

The local development workflow loop:

1. Write code
2. Run `gavel analyze --dir .`
3. Load the SARIF in your editor (or use `gavel review`)
4. Fix flagged issues
5. Re-run to verify

Each analysis produces a new result in `.gavel/results/`. Old results are kept for comparison.

## For Teams

- **Consistent rules**: Commit `.gavel/policies.yaml` to the repo so every developer sees the same findings
- **CI + local**: If you also set up [CI/PR gating](ci-pr-gating.md), the SARIF format is identical — you can download CI artifacts and open them in your editor locally
- **Personas**: Use `--persona security` for a security-focused review before a release, `--persona architect` for architecture reviews

## Tips

- **Fast iteration**: Use Ollama locally for instant feedback (1-3 sec/file), then let CI run a higher-quality model on the PR
- **Focus your review**: Use `--persona security` before cutting a release, `--persona architect` for design reviews
- **Review TUI**: `gavel review` provides a richer experience than raw SARIF viewing — try it alongside your editor

## Next Steps

- **Set up CI gating**: [CI/PR Gating Guide](ci-pr-gating.md)
- **Full configuration reference**: [README](../../README.md#configuration)
- **Custom rules**: [README - Custom Rules](../../README.md#custom-rules)
```

**Step 2: Review the guide for accuracy**

Verify:
- The `jq` command produces valid quickfix format from Gavel's SARIF structure
- The VS Code extension ID is correct
- The `.vscode/tasks.json` is valid JSON
- Links to other docs are correct

**Step 3: Commit**

```bash
git add docs/guides/editor-integration.md
git commit -m "docs: add editor integration user guide"
```

---

### Task 3: Update README with Guide Links

**Files:**
- Modify: `README.md` (add links in a new "Guides" section near the top)

**Step 1: Add a Guides section to the README**

After the "Quick Start" section and before "Usage", add:

```markdown
## Guides

- **[Gate PRs with AI Code Review](docs/guides/ci-pr-gating.md)** — Set up GitHub Actions to automatically analyze every PR
- **[See Findings in Your Editor](docs/guides/editor-integration.md)** — View Gavel findings inline in VS Code or Neovim
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: link to user guides from README"
```
