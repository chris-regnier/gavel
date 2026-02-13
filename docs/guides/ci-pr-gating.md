# Gate PRs with AI Code Review

By the end of this guide, every pull request in your repository will be analyzed by an AI code reviewer. Findings appear as native GitHub annotations on PR diffs via Code Scanning, and critical issues are flagged before anyone has to read a line of code.

## Prerequisites

- **Gavel binary** available as a [GitHub release](https://github.com/chris-regnier/gavel/releases) (the workflow downloads it automatically)
- A **GitHub repository** with Actions enabled
- An **LLM provider API key** (see Step 1)

## Step 1: Choose Your Provider

Pick the provider that matches your priority:

| Priority | Provider | Model | Approx. Cost per 100 Files |
|----------|----------|-------|-----------------------------|
| Speed | OpenRouter | `google/gemini-2.0-flash-exp` | ~$0.04 |
| Balance | Anthropic | `claude-haiku-4-5` | ~$2.40 |
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

Create a `.gavel/` directory in your repository root and add a `policies.yaml` file.

**For OpenRouter (recommended for most teams):**

```yaml
provider:
  name: openrouter
  openrouter:
    model: google/gemini-2.0-flash-exp

policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
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
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true
```

Commit this file to your repository. For other providers (Ollama, Bedrock, OpenAI), see [example-configs.yaml](../../example-configs.yaml).

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

      - name: Download Gavel
        run: |
          GAVEL_VERSION=$(curl -s https://api.github.com/repos/chris-regnier/gavel/releases/latest | jq -r '.tag_name')
          curl -sL "https://github.com/chris-regnier/gavel/releases/download/${GAVEL_VERSION}/gavel_${GAVEL_VERSION}_Linux_x86_64.tar.gz" | tar xz
          chmod +x gavel_Linux_x86_64
          sudo mv gavel_Linux_x86_64 /usr/local/bin/gavel

      - name: Get PR diff
        run: gh pr diff ${{ github.event.pull_request.number }} > pr.diff
        env:
          GH_TOKEN: ${{ github.token }}

      - name: Run Gavel analysis
        run: gavel analyze --diff pr.diff
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
          GH_TOKEN: ${{ github.token }}

      - name: Upload SARIF to GitHub Code Scanning
        uses: github/codeql-action/upload-sarif@v3
        if: always()
        with:
          sarif_file: .gavel/results/
```

### What each step does

1. **Checkout code** -- clones the repository so Gavel can read `.gavel/policies.yaml`.
2. **Download Gavel** -- fetches the latest release binary for Linux x86_64 from GitHub Releases.
3. **Get PR diff** -- uses `gh pr diff` to produce a unified diff of the pull request. The `GH_TOKEN` env var gives `gh` access to the repository.
4. **Run Gavel analysis** -- analyzes the diff against your configured policies. The provider API key is injected from the secret you created in Step 1. Results are written to `.gavel/results/<id>/sarif.json`.
5. **Upload SARIF** -- sends the SARIF file to GitHub Code Scanning. The `if: always()` ensures results are uploaded even if Gavel exits with a non-zero status (which happens on a `reject` verdict). The `security-events: write` permission declared at the top of the workflow is required for this step.

> **Using Anthropic instead of OpenRouter?** Change the env var in the "Run Gavel analysis" step to `ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}` and update your `.gavel/policies.yaml` accordingly.

## Step 4: Test It

1. Create a new branch and add a file with a known issue, for example:

```go
// main.go
package main

import "os/exec"

func main() {
    cmd := exec.Command("sh", "-c", os.Args[1]) // unsanitized input
    cmd.Run() // error ignored
}
```

2. Push the branch and open a pull request targeting `main`.
3. Watch the **Gavel AI Code Review** check appear in the PR checks section.
4. Once it completes, go to the **Security** tab > **Code scanning alerts** to see findings, or look for annotations directly on the **Files changed** tab of your PR.

## Step 5: Customize

### Add custom policies

Add more policies to `.gavel/policies.yaml`:

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
        run: gavel analyze --diff pr.diff --persona security
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
          GH_TOKEN: ${{ github.token }}
```

Available personas: `code-reviewer` (default), `architect`, `security`.

### Add custom rules

Place custom rule YAML files in `.gavel/rules/` in your repository. Gavel ships with 15 built-in rules (CWE, OWASP, SonarQube) and merges your custom rules on top. See the [custom rules section of the README](../../README.md#custom-rules) for the rule format.

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
| `gh pr diff` fails | Missing `GH_TOKEN` | Add `GH_TOKEN: ${{ github.token }}` to the env block of the step that runs `gh pr diff` |
| Binary download fails | Release asset name mismatch | Check the [releases page](https://github.com/chris-regnier/gavel/releases) for the exact tarball filename and update the download URL |

## Next Steps

- [Editor Integration Guide](./editor-integration.md) -- run Gavel from your editor for instant feedback before pushing
- [README Configuration Reference](../../README.md#configuration) -- full policy format, merging rules, and provider options
- [Custom Rules](../../README.md#custom-rules) -- write your own analysis rules with CWE/OWASP references
- [Custom Rego Policies](../../README.md#custom-rego-policies) -- fine-tune the gate logic beyond the default threshold
