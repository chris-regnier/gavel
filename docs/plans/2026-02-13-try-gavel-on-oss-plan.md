# Try Gavel on Open Source Code — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a docs/guides/ onboarding tutorial that walks new users through running Gavel on three public open-source repositories, demonstrating all three input modes (--dir, --files, --diff) across Go, Python, and JavaScript.

**Architecture:** Single markdown file at `docs/guides/try-on-open-source.md` following the structure and tone of the existing guides (`ci-pr-gating.md`, `editor-integration.md`). A link is added to the README.md Guides section.

**Tech Stack:** Markdown, Gavel CLI

---

### Task 1: Research target repositories

Verify the three target repos exist, are public, and identify the specific files/paths to use in the guide.

**Step 1: Verify repos and identify target files**

Check each repo on GitHub:

1. **Go — `0c34/govwa`**: Clone and list Go files. Identify 2-3 files likely to trigger findings (look for SQL queries, hardcoded passwords, empty error checks). The app is a vulnerable web app so most files should produce findings.

2. **Python — `OWASP/PyGoat`**: Clone and list Python files. Identify 2-3 files to recommend for `--files` mode. Look for views or models with hardcoded credentials, long functions, TODO comments.

3. **JS — `gothinkster/node-express-realworld-example-app`**: Clone and check `git log --oneline -10` to find a good commit range for diff mode. Identify commits that touch multiple files.

**Step 2: Document findings**

Record the specific files, commit SHAs, and expected rule triggers for each repo. These will be used in the guide's walkthrough sections.

**Step 3: Commit research notes (optional)**

If the research produces useful notes, append them to the design doc.

---

### Task 2: Write guide skeleton and prerequisites section

**Files:**
- Create: `docs/guides/try-on-open-source.md`

**Step 1: Write the guide skeleton**

Create the file with the title, intro paragraph, and all section headers:

```markdown
# Try Gavel on Open Source Code

By the end of this guide you will have run Gavel on three open-source repositories
in Go, Python, and JavaScript — using all three input modes.

## Prerequisites

## Example 1: Go — Directory Scan

## Example 2: Python — File-Targeted Scan

## Example 3: JavaScript — Diff Scan

## What's Next
```

**Step 2: Write the Prerequisites section**

Content:
- Gavel binary installed (link to releases page and README installation section)
- An LLM provider configured (link to Supported Providers in README)

**Step 3: Write the provider setup**

Provider table matching the CI guide style:

| Priority | Provider | Model | Setup |
|----------|----------|-------|-------|
| Free/local | Ollama | `qwen2.5-coder:7b` | `ollama pull qwen2.5-coder:7b` |
| Easy cloud | OpenRouter | `google/gemini-2.0-flash-exp` | API key from openrouter.ai |
| Best quality | Anthropic | `claude-haiku-4-5` | API key from console.anthropic.com |

Then show the three `.gavel/policies.yaml` variants (Ollama, OpenRouter, Anthropic) that the user will create in each example. Since every example uses the same config, show it once here and reference it from each example.

**Step 4: Commit**

```bash
git add docs/guides/try-on-open-source.md
git commit -m "docs: add skeleton and prerequisites for try-on-open-source guide"
```

---

### Task 3: Write Example 1 — Go directory scan

**Files:**
- Modify: `docs/guides/try-on-open-source.md`

**Step 1: Write the clone + configure steps**

```markdown
## Example 1: Go — Directory Scan

This example scans an entire Go web application for security and code quality issues.

### Clone the repository

\```bash
git clone https://github.com/0c34/govwa.git
cd govwa
\```

### Configure Gavel

\```bash
mkdir -p .gavel
\```

Then create `.gavel/policies.yaml` using the provider config from Prerequisites above.
```

**Step 2: Write the run command**

```markdown
### Run the analysis

\```bash
gavel analyze --dir .
\```
```

**Step 3: Write the annotated output walkthrough**

Show a realistic sample verdict JSON with annotations explaining each field:
- `decision` — what it means
- `reason` — summary
- `relevant_findings` — walk through 2-3 findings with their `gavel/confidence`, `gavel/explanation`, `gavel/recommendation`

Include a note: "Your exact findings may differ — the important thing is the workflow."

Use findings based on what the research in Task 1 reveals. If research hasn't happened yet, use plausible examples based on the known rule set (S3649 SQL injection, S2068 hardcoded credentials, S1086 error-ignored).

**Step 4: Write the "dig deeper" subsection**

Show how to find the full SARIF output:

```markdown
### Dig deeper

The full SARIF report is in `.gavel/results/`. Find the latest run:

\```bash
ls -t .gavel/results/ | head -1
\```

Open the `sarif.json` file to see all findings with full detail.
```

**Step 5: Write the persona switching subsection**

```markdown
### Try a different persona

Re-run the analysis with the security-focused persona:

\```bash
gavel analyze --dir . --persona security
\```

Compare the findings — the security persona emphasizes OWASP Top 10 vulnerabilities
and authentication issues.
```

**Step 6: Commit**

```bash
git add docs/guides/try-on-open-source.md
git commit -m "docs: add Example 1 (Go directory scan) to try-on-open-source guide"
```

---

### Task 4: Write Example 2 — Python file-targeted scan

**Files:**
- Modify: `docs/guides/try-on-open-source.md`

**Step 1: Write the clone + configure steps**

```markdown
## Example 2: Python — File-Targeted Scan

Instead of scanning an entire directory, target specific files you care about.

### Clone the repository

\```bash
git clone https://github.com/OWASP/PyGoat.git
cd PyGoat
\```

### Configure Gavel

\```bash
mkdir -p .gavel
\```

Create `.gavel/policies.yaml` (same as Example 1).
```

**Step 2: Write the run command with specific files**

Use the files identified in Task 1 research. Example:

```markdown
### Run the analysis

\```bash
gavel analyze --files introduction/views.py,authentication/views.py
\```

The `--files` flag takes a comma-separated list of file paths relative to the current directory.
```

**Step 3: Write the annotated output walkthrough**

Similar structure to Example 1 but shorter (no persona switching). Highlight different finding types — e.g., TODO comments (S1135), long functions (AST001), hardcoded credentials (S2068).

**Step 4: Commit**

```bash
git add docs/guides/try-on-open-source.md
git commit -m "docs: add Example 2 (Python file scan) to try-on-open-source guide"
```

---

### Task 5: Write Example 3 — JavaScript diff scan

**Files:**
- Modify: `docs/guides/try-on-open-source.md`

**Step 1: Write the clone + configure steps**

```markdown
## Example 3: JavaScript — Diff Scan

Analyze only the lines that changed — just like Gavel does in CI.

### Clone the repository

\```bash
git clone https://github.com/gothinkster/node-express-realworld-example-app.git
cd node-express-realworld-example-app
\```

### Configure Gavel

\```bash
mkdir -p .gavel
\```

Create `.gavel/policies.yaml` (same as before).
```

**Step 2: Write the diff generation and scan**

```markdown
### Generate a diff

Pick a range of commits to analyze:

\```bash
git log --oneline -10
\```

Generate a patch file from a commit range:

\```bash
git diff HEAD~3..HEAD > changes.patch
\```

### Run the analysis

\```bash
gavel analyze --diff changes.patch
\```

You can also pipe directly from git:

\```bash
git diff HEAD~3..HEAD | gavel analyze --diff -
\```
```

**Step 3: Write the annotated output walkthrough**

Shorter than Example 1. Emphasize that diff mode only analyzes changed lines, making it fast and focused — exactly what happens in CI.

**Step 4: Commit**

```bash
git add docs/guides/try-on-open-source.md
git commit -m "docs: add Example 3 (JS diff scan) to try-on-open-source guide"
```

---

### Task 6: Write the What's Next section

**Files:**
- Modify: `docs/guides/try-on-open-source.md`

**Step 1: Write the section**

```markdown
## What's Next

- **[Gate PRs with AI Code Review](ci-pr-gating.md)** — Automate Gavel in your CI pipeline with GitHub Actions
- **[See Findings in Your Editor](editor-integration.md)** — View findings inline in VS Code or Neovim
- **[Custom Rules](../../README.md#custom-rules)** — Write your own analysis rules
- **[Personas](../../README.md#personas)** — Switch between code-reviewer, architect, and security perspectives
```

**Step 2: Commit**

```bash
git add docs/guides/try-on-open-source.md
git commit -m "docs: add What's Next section to try-on-open-source guide"
```

---

### Task 7: Add guide link to README.md

**Files:**
- Modify: `README.md`

**Step 1: Add the link**

In the `## Guides` section of `README.md` (line 73-76), add the new guide as the first entry (since it's the "start here" guide):

```markdown
## Guides

- **[Try Gavel on Open Source Code](docs/guides/try-on-open-source.md)** — Run Gavel on real repositories in Go, Python, and JavaScript
- **[Gate PRs with AI Code Review](docs/guides/ci-pr-gating.md)** — Set up GitHub Actions to automatically analyze every PR
- **[See Findings in Your Editor](docs/guides/editor-integration.md)** — View Gavel findings inline in VS Code or Neovim
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add try-on-open-source guide link to README"
```

---

### Task 8: Final review pass

**Step 1: Read the complete guide**

Read `docs/guides/try-on-open-source.md` end-to-end and check:
- All links work (relative paths are correct)
- Provider configs are consistent across examples
- Sample output is plausible
- Tone matches existing guides
- No broken markdown

**Step 2: Fix any issues found**

**Step 3: Commit fixes if any**

```bash
git add docs/guides/try-on-open-source.md
git commit -m "docs: polish try-on-open-source guide"
```
