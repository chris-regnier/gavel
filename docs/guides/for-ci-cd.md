# For CI/CD

Automated AI code review on every pull request. Traditional code review is a bottleneck — Gavel gives every PR instant, consistent feedback so human reviewers focus on design, not catching bugs.

## What It Looks Like

When a PR is opened, Gavel:

1. Analyzes the diff against your configured policies
2. Runs 19 built-in rules instantly (regex + tree-sitter AST)
3. Sends findings to GitHub Code Scanning as native annotations on the PR diff
4. Posts a verdict in the job summary: **merge**, **reject**, or **review**

High-confidence issues block the merge. Everything else gets flagged for human review. No findings? Auto-merge.

## Cost at Scale

| Model | Speed | Cost per 100 Files | Best For |
|-------|-------|--------------------|----------|
| `google/gemini-2.0-flash-exp` (OpenRouter) | ~2-5 sec/file | ~$0.20 | Most PRs — fast and cheap |
| `deepseek/deepseek-chat` (OpenRouter) | ~3-5 sec/file | ~$0.21 | Budget-conscious teams |
| `claude-haiku-4-5` (Anthropic) | ~3-6 sec/file | ~$2.40 | Quality balance |
| `claude-sonnet-4` (Anthropic) | ~5-15 sec/file | ~$9.00 | Release branches, deep review |

**Recommendation:** Use a fast, cheap model for most PRs. Only run the expensive model on PRs targeting release branches or tagged for deeper review.

## Set Up GitHub Actions

### Step 1: Add your API key as a secret

1. Go to your repository on GitHub
2. **Settings** > **Secrets and variables** > **Actions**
3. **New repository secret**: `OPENROUTER_API_KEY` (or `ANTHROPIC_API_KEY`)

### Step 2: Add a policies config

Create `.github/policies.yaml` (or `.gavel/policies.yaml`):

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

### Step 3: Add the workflow

Create `.github/workflows/gavel.yml`:

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

          echo "Gavel decision: ${DECISION}"

      - name: Upload SARIF to GitHub Code Scanning
        uses: github/codeql-action/upload-sarif@v4
        if: always() && env.SKIP_ANALYSIS != 'true'
        with:
          sarif_file: /tmp/gavel-sarif/
          category: gavel
```

### Step 4: Test it

Push a branch with a known issue (e.g., unsanitized user input, hardcoded secret) and open a PR. The **Gavel AI Code Review** check will appear in the PR checks section.

## Enable Auto-Merge

If the verdict is not "reject", automatically merge clean PRs. Add these permissions:

```yaml
permissions:
  contents: write
  pull-requests: write
  security-events: write
```

And add this step at the end of the job:

```yaml
      - name: Auto-merge
        if: env.SKIP_ANALYSIS != 'true'
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          DECISION=$(jq -r '.decision' /tmp/verdict.json)
          if [ "$DECISION" != "reject" ]; then
            echo "Gavel passed - enabling auto-merge"
            gh pr merge "${{ github.event.pull_request.number }}" --auto --squash
          fi
```

Auto-merge must be enabled in repository settings (**Settings** > **General** > **Pull Requests** > **Allow auto-merge**).

## Scale Across Repositories

### Shared configuration

- **Per-project:** Commit `.gavel/policies.yaml` to each repository
- **Org-wide defaults:** Place `~/.config/gavel/policies.yaml` on self-hosted runners for base policies and provider config that apply everywhere unless overridden
- **Org-wide rules:** `~/.config/gavel/rules/*.yaml` works the same way

### Tiered model strategy

Use different models based on PR context:

```yaml
# .github/workflows/gavel.yml - conditional model selection
      - name: Select model
        run: |
          if [[ "${{ github.base_ref }}" == "release/"* ]]; then
            echo "MODEL=anthropic/claude-sonnet-4" >> "$GITHUB_ENV"
          else
            echo "MODEL=google/gemini-2.0-flash-exp" >> "$GITHUB_ENV"
          fi
```

- **Most PRs:** Fast, cheap model (Gemini Flash, DeepSeek)
- **Release branches:** Thorough model (Sonnet, GPT-5)
- **Security-sensitive:** Add `--persona security` for OWASP-focused analysis

### Cache for cost savings

Gavel uses deterministic cache keys (`gavel/cache_key` in SARIF). If the same file was already analyzed with the same model and policies, you can skip re-analysis. This is especially useful for monorepos where most files don't change per PR.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `OPENROUTER_API_KEY not set` | Verify the secret exists in Settings > Secrets and that the workflow step has the `env:` block |
| Analysis times out | Switch to a faster model or increase `timeout-minutes:` |
| `model not found` | Check model string matches provider format (OpenRouter: `org/model`, Anthropic: just the name) |
| No annotations on PR | Go to Settings > Code security and analysis > Code scanning and confirm it's enabled |
| SARIF upload fails with 403 | Ensure `security-events: write` is in the `permissions:` block |
| Empty diff | Ensure `fetch-depth: 0` on the checkout step |

## Next Steps

- **[Customize policies](../configuration/policies.md)** — add project-specific rules
- **[Custom Rego](../configuration/rego.md)** — fine-tune the gate threshold
- **[Editor Integration](editor-integration.md)** — view CI findings locally in VS Code
- **[For Security Teams](for-security-teams.md)** — OWASP-focused analysis for security-sensitive repos
