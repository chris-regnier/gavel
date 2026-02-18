# Try Gavel on Open Source Code

By the end of this guide you will have run Gavel on three open-source repositories in Go, Python, and TypeScript -- using all three input modes.

Each example uses a deliberately vulnerable or well-known project so that Gavel produces real findings you can inspect. You will learn the three ways to feed code into Gavel (`--dir`, `--files`, `--diff`) and how to read the output.

## Prerequisites

- **Gavel binary installed** -- download from the [releases page](https://github.com/chris-regnier/gavel/releases) or follow the [README installation instructions](../../README.md#installation)
- **An LLM provider** -- pick the one that fits your situation:

| Priority | Provider | Model | Setup |
|----------|----------|-------|-------|
| Free/local | Ollama | `qwen2.5-coder:7b` | `ollama pull qwen2.5-coder:7b` |
| Easy cloud | OpenRouter | `google/gemini-2.0-flash-exp` | API key from [openrouter.ai](https://openrouter.ai) |
| Best quality | Anthropic | `claude-haiku-4-5` | API key from [console.anthropic.com](https://console.anthropic.com) |

### Provider configuration

Every example in this guide needs a `.gavel/policies.yaml` file in the repository you are analyzing. Pick the config block for your provider and use it in all three examples.

**Ollama (free, local):**

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

If you use Ollama, make sure the server is running (`ollama serve`) and you have pulled the model (`ollama pull qwen2.5-coder:7b`) before continuing.

**OpenRouter:**

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

Set your API key before running Gavel:

```bash
export OPENROUTER_API_KEY="your-key-here"
```

**Anthropic:**

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

Set your API key before running Gavel:

```bash
export ANTHROPIC_API_KEY="your-key-here"
```

With your provider chosen and key exported, you are ready to analyze some code.

---

## Example 1: Go -- Directory Scan

**Repo:** [0c34/govwa](https://github.com/0c34/govwa) -- Go Vulnerable Web Application, a deliberately insecure web app with ~20 Go files.

**Input mode:** `--dir` scans every file in a directory tree.

### Clone the repo

```bash
git clone https://github.com/0c34/govwa.git
cd govwa
```

### Configure Gavel

```bash
mkdir -p .gavel
```

Create `.gavel/policies.yaml` with the config block for your chosen provider from the [prerequisites section](#provider-configuration).

### Run the analysis

```bash
gavel analyze --dir .
```

Gavel reads every Go file in the repository, sends each one to your LLM provider along with the enabled policies, and prints an analysis summary to stdout.

### Read the output

The summary tells you the analysis ID and how many findings were detected. Here is a plausible output for this repository:

```json
{
  "id": "2026-02-18T14-30-00Z-a1b2c3",
  "findings": 5,
  "scope": "directory",
  "persona": "code-reviewer"
}
```

To get the gate verdict, run `gavel judge`:

```bash
gavel judge
```

This evaluates the SARIF with Rego policies and prints a verdict -- whether this code should be merged, reviewed, or rejected:

```json
{
  "decision": "reject",
  "reason": "Decision: reject based on 5 findings",
  "relevant_findings": [
    {
      "ruleId": "S3649",
      "level": "error",
      "message": {
        "text": "Possible SQL injection vulnerability: raw user input interpolated into SQL query via fmt.Sprintf"
      },
      "locations": [
        {
          "physicalLocation": {
            "artifactLocation": {
              "uri": "vulnerability/sqli/function.go"
            },
            "region": {
              "startLine": 14
            }
          }
        }
      ],
      "properties": {
        "gavel/confidence": 0.95,
        "gavel/explanation": "The function UnsafeQueryGetData builds a SQL query using fmt.Sprintf with user-supplied uid directly interpolated into the query string, allowing SQL injection.",
        "gavel/recommendation": "Use parameterized queries with placeholder arguments instead of string interpolation."
      }
    }
  ]
}
```

Here is what each part means:

- **`decision`** -- the gate verdict from `gavel judge`. `"reject"` means at least one finding crossed the confidence and severity threshold. Other possible values are `"merge"` (no findings) and `"review"` (findings exist but none are severe enough to auto-reject).
- **`reason`** -- a human-readable summary of why the decision was made.
- **`relevant_findings`** -- the SARIF results that drove the decision. Each finding includes:
  - **`ruleId`** -- the rule that matched (here `S3649`, a SonarQube SQL injection rule).
  - **`level`** -- severity: `error`, `warning`, or `note`.
  - **`message.text`** -- what the AI found.
  - **`locations`** -- where in the code the issue appears, with file path and line number.
  - **`properties`** -- Gavel-specific metadata under the `gavel/` prefix:
    - **`gavel/confidence`** -- how confident the AI is in this finding (0.0 to 1.0).
    - **`gavel/explanation`** -- the AI's reasoning for flagging this code.
    - **`gavel/recommendation`** -- a concrete suggestion for fixing the issue.

> Your exact findings will differ depending on your provider and model. The structure is always the same.

### Dig deeper

Gavel writes the full SARIF log and verdict to `.gavel/results/`. Find the latest run:

```bash
ls -t .gavel/results/ | head -1
```

Open the SARIF file to see every finding, not just the ones that drove the verdict:

```bash
cat .gavel/results/$(ls -t .gavel/results/ | head -1)/sarif.json | jq .
```

The SARIF file is a standard [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) log. You can open it in VS Code with the [SARIF Viewer extension](https://marketplace.visualstudio.com/items?itemName=MS-SarifVSCode.sarif-viewer) or process it with any SARIF-compatible tool.

### Try a different persona

Re-run with the security persona to get findings focused on OWASP Top 10 and authentication issues:

```bash
gavel analyze --dir . --persona security
```

The security persona shifts the AI's focus toward injection, broken authentication, sensitive data exposure, and other security-specific concerns. Compare the output with the default `code-reviewer` persona to see how the perspective changes.

---

## Example 2: Python -- File-Targeted Scan

**Repo:** [adeyosemanputra/pygoat](https://github.com/adeyosemanputra/pygoat) -- a Django-based intentionally vulnerable application for learning about web security.

**Input mode:** `--files` analyzes specific files instead of an entire directory. This is useful when you know which files you want to check or when a full directory scan would be too slow.

### Clone the repo

```bash
git clone https://github.com/adeyosemanputra/pygoat.git
cd pygoat
```

### Configure Gavel

```bash
mkdir -p .gavel
```

Create `.gavel/policies.yaml` with your chosen provider config from the [prerequisites section](#provider-configuration).

### Run the analysis

```bash
gavel analyze --files pygoat/settings.py,introduction/views.py
```

The `--files` flag takes a comma-separated list of paths relative to the current directory. Gavel only analyzes the files you specify -- nothing else in the repository is touched.

### Read the output

After running `gavel judge`, a plausible verdict for these two files:

```json
{
  "decision": "reject",
  "reason": "Decision: reject based on 3 findings",
  "relevant_findings": [
    {
      "ruleId": "S2068",
      "level": "error",
      "message": {
        "text": "Hardcoded secret: Django SECRET_KEY is set to a static string in source code"
      },
      "locations": [
        {
          "physicalLocation": {
            "artifactLocation": { "uri": "pygoat/settings.py" },
            "region": { "startLine": 23 }
          }
        }
      ],
      "properties": {
        "gavel/confidence": 0.97,
        "gavel/explanation": "The SECRET_KEY setting is hardcoded as a string literal. This key is used for cryptographic signing in Django and must not be committed to version control.",
        "gavel/recommendation": "Load SECRET_KEY from an environment variable or a secrets manager."
      }
    },
    {
      "ruleId": "S2068",
      "level": "error",
      "message": {
        "text": "Hardcoded credentials: DEBUG mode enabled with default database password"
      },
      "locations": [
        {
          "physicalLocation": {
            "artifactLocation": { "uri": "pygoat/settings.py" },
            "region": { "startLine": 26 }
          }
        }
      ],
      "properties": {
        "gavel/confidence": 0.92,
        "gavel/explanation": "DEBUG is set to True and database credentials are hardcoded. In production this exposes detailed error pages and uses known credentials.",
        "gavel/recommendation": "Set DEBUG via an environment variable and use a secrets manager for database credentials."
      }
    }
  ]
}
```

Notice that both findings use the same rule ID (`S2068` -- hardcoded credentials) but flag different locations and provide distinct explanations. The AI treats each occurrence independently.

> Your exact findings will differ depending on your provider and model. The structure is always the same.

### Dig deeper

Same pattern as before -- find the latest results:

```bash
ls -t .gavel/results/ | head -1
```

---

## Example 3: TypeScript -- Diff Scan

**Repo:** [gothinkster/node-express-realworld-example-app](https://github.com/gothinkster/node-express-realworld-example-app) -- an Express.js implementation of the RealWorld API spec.

**Input mode:** `--diff` analyzes only the lines changed in a unified diff. This is the fastest and most focused mode -- it is how Gavel works in CI, where `gh pr diff` pipes into `gavel analyze --diff -`.

### Clone the repo

```bash
git clone https://github.com/gothinkster/node-express-realworld-example-app.git
cd node-express-realworld-example-app
```

### Configure Gavel

```bash
mkdir -p .gavel
```

Create `.gavel/policies.yaml` with your chosen provider config from the [prerequisites section](#provider-configuration).

### Generate a diff

To use diff mode, you need a unified diff. The easiest way is to generate one from the git history:

```bash
# See recent commits
git log --oneline -10

# Generate a patch from the last 5 commits
git diff HEAD~5..HEAD > changes.patch
```

If the repository has fewer than 5 commits, adjust the range (e.g., `HEAD~2..HEAD`). You can also diff between any two branches or tags.

### Run the analysis

```bash
gavel analyze --diff changes.patch
```

Gavel parses the unified diff, extracts only the changed hunks, and sends those to the LLM. Files that were not modified are ignored entirely.

### Pipe directly from git

You can skip the intermediate file and pipe the diff into Gavel via stdin:

```bash
git diff HEAD~5..HEAD | gavel analyze --diff -
```

The `-` argument tells Gavel to read the diff from stdin. This is the exact pattern used in CI workflows:

```bash
gh pr diff 42 | gavel analyze --diff -
gavel judge
```

### Read the output

Because diff mode only analyzes changed lines, you will typically see fewer findings than a full directory scan. Run `gavel judge` to get the verdict:

```json
{
  "decision": "review",
  "reason": "Decision: review based on 2 findings",
  "relevant_findings": [
    {
      "ruleId": "shall-be-merged",
      "level": "warning",
      "message": {
        "text": "Error from database query is not checked before using the result"
      },
      "locations": [
        {
          "physicalLocation": {
            "artifactLocation": { "uri": "src/models/User.ts" },
            "region": { "startLine": 45 }
          }
        }
      ],
      "properties": {
        "gavel/confidence": 0.78,
        "gavel/explanation": "The return value of the database query is used directly without checking for errors or null results.",
        "gavel/recommendation": "Add error handling around the query and validate the result before accessing its properties."
      }
    }
  ]
}
```

Notice the `"review"` decision instead of `"reject"` -- the confidence is below the 0.8 threshold, so Gavel flags it for human review rather than blocking the merge.

> Your exact findings will differ depending on your provider and model. The structure is always the same.

### Why diff mode matters

Diff mode is how Gavel delivers fast, focused PR reviews:

- **Speed** -- only changed lines are analyzed, so even a large repository finishes quickly.
- **Relevance** -- findings are scoped to the code that actually changed, reducing noise.
- **CI integration** -- the `gh pr diff | gavel analyze --diff -` followed by `gavel judge` pattern works in any CI workflow. See the [CI/PR Gating Guide](ci-pr-gating.md) for the full setup.

---

## What You Have Learned

You have now used all three Gavel input modes:

| Mode | Flag | Best For |
|------|------|----------|
| Directory scan | `--dir .` | Full project analysis, onboarding to a new codebase |
| File-targeted | `--files a.py,b.py` | Checking specific files you are working on |
| Diff scan | `--diff changes.patch` | PR review, CI pipelines, incremental analysis |

Each mode produces the same SARIF output and verdict format. The only difference is what code gets sent to the LLM.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `OPENROUTER_API_KEY not set` | API key not exported in your shell | Run `export OPENROUTER_API_KEY="your-key"` before `gavel analyze` |
| `ANTHROPIC_API_KEY not set` | API key not exported in your shell | Run `export ANTHROPIC_API_KEY="your-key"` before `gavel analyze` |
| `connection refused` on port 11434 | Ollama server is not running | Start it with `ollama serve` |
| `model not found` | Model not pulled or wrong name | Run `ollama pull qwen2.5-coder:7b` (Ollama) or check the model string matches your provider |
| No output or empty findings | The LLM did not flag anything | Try a different model or add more specific policy instructions |
| `--diff` produces no findings | Diff range has no code changes | Check that `git diff HEAD~5..HEAD` actually produces output before piping to Gavel |
| Analysis is slow | Large files or slow model | Use `--files` to narrow scope, or switch to a faster model (`gemini-2.0-flash-exp`, `qwen2.5-coder:7b`) |

## What's Next

- **[Gate PRs with AI Code Review](ci-pr-gating.md)** -- automate Gavel in your CI pipeline with GitHub Actions
- **[See Findings in Your Editor](editor-integration.md)** -- view findings inline in VS Code or Neovim
- **[Custom Rules](../../README.md#custom-rules)** -- write your own analysis rules
- **[Personas](../../README.md#personas)** -- switch between code-reviewer, architect, and security perspectives
