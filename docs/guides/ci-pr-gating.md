# Gate PRs with AI Code Review

By the end of this guide, every pull request in your repository will be analyzed by an AI code reviewer. Findings appear as native GitHub annotations on PR diffs via Code Scanning, and a verdict (merge, reject, or review) is reported in the job summary.

## Prerequisites

- **Gavel binary** available as a [GitHub release](https://github.com/chris-regnier/gavel/releases) (the workflow downloads it automatically)
- A **GitHub repository** with Actions enabled
- An **LLM provider API key** (see Step 1)

## Step 1: Choose Your Provider

Pick the provider that matches your priority:

| Priority | Provider | Model | Approx. Cost per 100 Files |
|----------|----------|-------|-----------------------------|
| Speed | OpenRouter | `google/gemini-2.0-flash-exp` | ~$0.04 |
| Balance | OpenRouter | `anthropic/claude-haiku-4-5` | ~$2.40 |
| Quality | Anthropic | `claude-sonnet-4-5` | ~$18.00 |
| Budget | OpenRouter | `deepseek/deepseek-chat` | ~$0.20 |

**OpenRouter** and **Anthropic** are the easiest providers for CI because they only need a single API key -- no AWS credentials, no local server.

### Add the API key as a GitHub Actions secret

1. Open your repository on GitHub.
2. Go to **Settings** > **Secrets and variables** > **Actions**.
3. Click **New repository secret**.
4. Set the name to `OPENROUTER_API_KEY` (or `ANTHROPIC_API_KEY` if using Anthropic directly).
5. Paste your API key as the value and click **Add secret**.

## Step 2: Add Project Config

Create a policies config file for CI. You can place it anywhere -- the workflow tells Gavel where to find it with `--policies`. A common convention is `.github/policies.yaml` to keep CI config together, or `.gavel/policies.yaml` for the project default.

**For OpenRouter (recommended for most teams):**

```yaml
provider:
  name: openrouter
  openrouter:
    model: anthropic/claude-haiku-4-5

policies:
  shall-be-merged:
    severity: error
    enabled: true
```

**For Anthropic:**

```yaml
provider:
  name: anthropic
  anthropic:
    model: claude-haiku-4-5

policies:
  shall-be-merged:
    severity: error
    enabled: true
```

Commit this file to your repository. For other providers (Ollama, Bedrock, OpenAI), see [example-configs.yaml](https://github.com/chris-regnier/gavel/blob/main/example-configs.yaml).

## Step 3: Add the GitHub Actions Workflow

Create `.github/workflows/gavel.yml` in your repository:

```yaml
name: Gavel AI Code Review

on:
  pull_request:
    branches: [main]

permissions:
  contents: read
  security-events: write

jobs:
  gavel:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Get PR diff
        run: |
          git diff origin/${{ github.base_ref }}...HEAD -- ':!docs/' > /tmp/pr.diff
          echo "--- Diff stats ---"
          wc -l /tmp/pr.diff
          if [ ! -s /tmp/pr.diff ]; then
            echo "Empty diff, skipping analysis"
            echo "SKIP_ANALYSIS=true" >> "$GITHUB_ENV"
          fi

      - name: Download latest Gavel release
        if: env.SKIP_ANALYSIS != 'true'
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          BINARY_NAME="gavel_Linux_x86_64"
          gh release download --repo chris-regnier/gavel \
            --pattern "gavel_*_Linux_x86_64.tar.gz" --dir /tmp || true

          TARBALL=$(ls /tmp/gavel_*_Linux_x86_64.tar.gz 2>/dev/null | head -1)
          if [ -z "$TARBALL" ]; then
            echo "::warning::No released Gavel binary found. Skipping analysis."
            echo "SKIP_ANALYSIS=true" >> "$GITHUB_ENV"
            exit 0
          fi

          tar -xzf "$TARBALL" -C /tmp
          chmod +x "/tmp/${BINARY_NAME}"
          mv "/tmp/${BINARY_NAME}" /tmp/gavel
          /tmp/gavel version

      - name: Run Gavel analysis
        if: env.SKIP_ANALYSIS != 'true'
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
        run: |
          /tmp/gavel analyze \
            --diff /tmp/pr.diff \
            --policies .github \
            --output /tmp/gavel-results

      - name: Run Gavel judge
        if: env.SKIP_ANALYSIS != 'true'
        run: |
          /tmp/gavel judge --output /tmp/gavel-results

      - name: Collect results
        if: env.SKIP_ANALYSIS != 'true'
        run: |
          mkdir -p /tmp/gavel-sarif

          RESULT_DIR=$(ls -td /tmp/gavel-results/*/ 2>/dev/null | head -1)
          if [ -z "$RESULT_DIR" ]; then
            echo "::error::No Gavel result directory found"
            exit 1
          fi

          if [ -f "$RESULT_DIR/verdict.json" ]; then
            cp "$RESULT_DIR/verdict.json" /tmp/verdict.json
          else
            echo '{"decision":"merge","reason":"No findings produced by analysis"}' > /tmp/verdict.json
          fi

          if [ -f "$RESULT_DIR/sarif.json" ]; then
            cp "$RESULT_DIR/sarif.json" /tmp/gavel-sarif/gavel.sarif
          fi

      - name: Evaluate verdict
        if: env.SKIP_ANALYSIS != 'true'
        run: |
          DECISION=$(jq -r '.decision' /tmp/verdict.json)
          REASON=$(jq -r '.reason // "No reason provided"' /tmp/verdict.json)

          echo "## Gavel Verdict" >> "$GITHUB_STEP_SUMMARY"
          echo "" >> "$GITHUB_STEP_SUMMARY"
          echo "**Decision:** \`${DECISION}\`" >> "$GITHUB_STEP_SUMMARY"
          echo "**Reason:** ${REASON}" >> "$GITHUB_STEP_SUMMARY"

          if [ "$DECISION" = "reject" ]; then
            echo "" >> "$GITHUB_STEP_SUMMARY"
            echo "### Relevant Findings" >> "$GITHUB_STEP_SUMMARY"
            jq -r '.relevant_findings[]? | "- **\(.ruleId // "unknown")** (\(.level // "note")): \(.message.text // "No message")"' \
              /tmp/verdict.json >> "$GITHUB_STEP_SUMMARY" 2>/dev/null || true
            echo "::error::Gavel rejected this PR: ${REASON}"
            exit 1
          fi

          echo "✅ Gavel decision: ${DECISION}"

      - name: Upload SARIF to GitHub Code Scanning
        uses: github/codeql-action/upload-sarif@v4
        if: always() && env.SKIP_ANALYSIS != 'true'
        with:
          sarif_file: /tmp/gavel-sarif/
          category: gavel
```

### What each step does

1. **Checkout code** -- clones the repository with full history (`fetch-depth: 0`) so `git diff` can compute the PR diff.
2. **Get PR diff** -- uses `git diff` against the base branch to produce a unified diff, excluding directories you don't want reviewed (e.g. `docs/`). If the diff is empty the remaining steps are skipped.
3. **Download latest Gavel release** -- uses `gh release download` to fetch the latest release binary. If no release exists yet, the job exits cleanly with a warning.
4. **Run Gavel analysis** -- analyzes the diff against your configured policies. The `--policies` flag points to the directory containing `policies.yaml`. The `--output` flag writes results to `/tmp/gavel-results` instead of the default `.gavel/results/`, keeping the workspace clean. The provider API key is injected from the secret you created in Step 1.
5. **Run Gavel judge** -- evaluates the SARIF results with Rego policies to produce a verdict (merge, reject, or review). The `--output` flag must match the one used in the analysis step.
6. **Collect results** -- copies the verdict and SARIF from the timestamped result directory into known paths for the remaining steps. Defaults to a "merge" verdict if no findings were produced.
7. **Evaluate verdict** -- writes the verdict to the GitHub Actions job summary. If the decision is "reject", the step fails the workflow and lists the relevant findings.
8. **Upload SARIF** -- sends the SARIF file to GitHub Code Scanning using the CodeQL upload action (v4). The `if: always()` condition ensures results are uploaded even if the verdict step fails. The `category: gavel` prevents collisions with other SARIF-producing tools. The `security-events: write` permission declared at the top of the workflow is required for this step.

> **Using Anthropic instead of OpenRouter?** Change the env var in the "Run Gavel analysis" step to `ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}` and update your `policies.yaml` accordingly.

## Step 4: Test It

1. Create a new branch and add a file with a known issue, for example:

```go
// main.go
package main

import (
    "os"
    "os/exec"
)

func main() {
    cmd := exec.Command("sh", "-c", os.Args[1]) // unsanitized input
    cmd.Run() // error ignored
}
```

2. Push the branch and open a pull request targeting `main`.
3. Watch the **Gavel AI Code Review** check appear in the PR checks section.
4. Once it completes, click the job to see the **Gavel Verdict** in the job summary. Go to the **Security** tab > **Code scanning alerts** to see findings as annotations on the **Files changed** tab of your PR.

## Step 5: Customize

### Add custom policies

Add more policies to your `policies.yaml`:

```yaml
policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true

  no-hardcoded-secrets:
    description: "No hardcoded secrets"
    severity: error
    instruction: "Flag any hardcoded API keys, passwords, tokens, or connection strings."
    enabled: true

  function-length:
    description: "Functions should not exceed a reasonable length"
    severity: note
    instruction: "Flag functions longer than 50 lines."
    enabled: true
```

### Use a specialized persona

Run analysis with a security-focused reviewer by adding the `--persona` flag to your workflow:

```yaml
      - name: Run Gavel analysis
        if: env.SKIP_ANALYSIS != 'true'
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
        run: |
          /tmp/gavel analyze \
            --diff /tmp/pr.diff \
            --policies .github \
            --output /tmp/gavel-results \
            --persona security
```

Available personas: `code-reviewer` (default), `code-reviewer-verbose`, `architect`, `security`, `research-assistant`, `sharp-editor`.

### Add custom rules

Place custom rule YAML files in `.gavel/rules/` in your repository. Gavel ships with 19 built-in rules (CWE, OWASP, SonarQube) and merges your custom rules on top. See the [custom rules documentation](configuration/policies.md#custom-rules) for the rule format.

### Adjust the gate threshold

By default, Gavel rejects PRs when any error-level finding has confidence above 0.8. To only reject on very high confidence findings, create `.gavel/rego/custom.rego`:

```rego
package gavel.gate

import rego.v1

default decision := "review"

decision := "reject" if {
    some result in input.runs[0].results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.9
}

decision := "merge" if {
    count(input.runs[0].results) == 0
}
```

Custom `.rego` files in the rego directory override the built-in gate policy entirely.

### Enable auto-merge

If Gavel's verdict is not "reject", you can automatically merge the PR by adding a step after the SARIF upload. This requires upgrading the workflow permissions to `contents: write` and `pull-requests: write`:

```yaml
permissions:
  contents: write
  pull-requests: write
  security-events: write
```

Then add the step at the end of the job:

```yaml
      - name: Auto-merge
        if: env.SKIP_ANALYSIS != 'true'
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          DECISION=$(jq -r '.decision' /tmp/verdict.json)
          if [ "$DECISION" != "reject" ]; then
            echo "Gavel passed — enabling auto-merge"
            gh pr merge "${{ github.event.pull_request.number }}" --auto --squash
          fi
```

This uses GitHub's auto-merge feature, which waits for all other required status checks to pass before merging. To restrict auto-merge to specific PR authors (e.g. bots or trusted contributors), add a condition to the step:

```yaml
      - name: Auto-merge
        if: >-
          env.SKIP_ANALYSIS != 'true' &&
          github.event.pull_request.user.login == 'my-bot'
```

> **Note:** Auto-merge must be enabled in your repository settings (**Settings** > **General** > **Pull Requests** > **Allow auto-merge**) for the `gh pr merge --auto` command to work.

## For Teams

> **Shared configuration across repositories**
>
> - Commit `.gavel/policies.yaml` to each repository for project-specific policies.
> - Use the machine-level config at `~/.config/gavel/policies.yaml` on self-hosted runners to set org-wide defaults (provider, base policies) that apply to every project unless overridden.
> - Machine-level rules at `~/.config/gavel/rules/*.yaml` work the same way -- define org-wide rules once, override per-project as needed.
>
> **Cost at scale**
>
> - Use a fast, cheap model for most PRs (`google/gemini-2.0-flash-exp` or `deepseek/deepseek-chat`).
> - Only run the expensive model (`claude-sonnet-4-5`, `claude-opus-4-6`) on PRs targeting release branches or tagged for deeper review.
> - Gavel uses deterministic cache keys (`gavel/cache_key` in SARIF). If the same file was already analyzed with the same model and policies, you can skip re-analysis.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `OPENROUTER_API_KEY not set` or `ANTHROPIC_API_KEY not set` | Secret not configured or env var not passed to the step | Verify the secret exists in Settings > Secrets and that the workflow step has the correct `env:` block |
| Analysis times out | Large diff or slow model | Switch to a faster model (`gemini-2.0-flash-exp`, `claude-haiku-4-5`) or increase the Actions job timeout with `timeout-minutes:` |
| `model not found` | Wrong model name for the provider | Check the model string matches your provider exactly -- OpenRouter models use `org/model` format, Anthropic uses just the model name |
| No annotations on the PR | SARIF upload succeeded but Code Scanning is not enabled | Go to Settings > Code security and analysis > Code scanning and confirm it is enabled for your repository |
| SARIF upload fails with 403 | Missing permission | Ensure the workflow has `security-events: write` in the `permissions:` block |
| `git diff` produces empty diff | Shallow clone | Ensure `fetch-depth: 0` is set on the checkout step so full history is available |
| Binary download fails or warns "No released Gavel binary found" | No release exists yet or asset name mismatch | Check the [releases page](https://github.com/chris-regnier/gavel/releases) for the exact tarball filename; the workflow uses `gh release download` with a glob pattern |

## Next Steps

- [Editor Integration Guide](./editor-integration.md) -- run Gavel from your editor for instant feedback before pushing
- [Policies & Rules](configuration/policies.md) -- full policy format, merging rules, and custom analysis rules
- [Custom Rego Policies](configuration/rego.md) -- fine-tune the gate logic beyond the default threshold
- [Providers](PROVIDERS.md) -- all provider options and configuration
