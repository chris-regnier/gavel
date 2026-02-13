# See Gavel Findings in Your Editor

Run Gavel locally and see AI findings as inline diagnostics, just like a built-in linter. This guide walks you through configuring Gavel, running an analysis, and viewing the results in VS Code or Neovim.

## Prerequisites

- **Gavel installed** -- see the [README installation instructions](../../README.md#installation)
- **LLM provider configured** -- Ollama (recommended for local dev) or a cloud provider API key
- **VS Code** or **Neovim**

## Step 1: Configure Gavel Locally

Create a `.gavel/` directory in your project root with a `policies.yaml` file.

**Ollama is the best choice for local development** -- it runs on your machine, costs nothing, and responds fast enough for iterative use.

### Install Ollama

```sh
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh
```

Start the Ollama server and pull a fast coding model:

```sh
ollama serve &
ollama pull qwen2.5-coder:7b
```

### Create the config file

```sh
mkdir -p .gavel
```

Write `.gavel/policies.yaml`:

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

If you prefer a cloud provider (Anthropic, OpenRouter, OpenAI, Bedrock), see [example-configs.yaml](../../example-configs.yaml) for complete configuration examples. Cloud providers require an API key set as an environment variable.

## Step 2: Run an Analysis

Analyze a directory:

```sh
gavel analyze --dir ./src
```

Or analyze specific files:

```sh
gavel analyze --files main.go,handler.go
```

Gavel prints a JSON verdict to stdout and writes two files under `.gavel/results/`:

```
.gavel/results/
  2026-02-13T14-30-00Z-a1b2c3/
    sarif.json       # SARIF 2.1.0 log with all findings
    verdict.json     # Gate decision: merge, review, or reject
```

The verdict looks like this:

```json
{
  "decision": "review",
  "reason": "findings require human review"
}
```

The SARIF file contains every finding with its location, message, explanation, recommendation, and confidence score. This is the file your editor will consume.

## Step 3a: VS Code

### Install the SARIF Viewer extension

Open VS Code and install the Microsoft SARIF Viewer:

1. Open the Extensions panel (`Ctrl+Shift+X` / `Cmd+Shift+X`).
2. Search for `MS-SarifVSCode.sarif-viewer`.
3. Click **Install**.

Or install from the command line:

```sh
code --install-extension MS-SarifVSCode.sarif-viewer
```

### Open a SARIF log

1. Run `gavel analyze --dir .` from your project root.
2. Open the Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`).
3. Type **SARIF: Open SARIF Log** and select it.
4. Navigate to `.gavel/results/`, pick the latest timestamped directory, and open `sarif.json`.

Findings appear in three places:

- **Problems panel** -- each finding listed with file, line, and severity
- **Inline squiggles** -- underlines on the affected lines in the editor
- **Hover tooltips** -- hover over a squiggle to see the full message, including the AI explanation and recommendation from the `gavel/explanation` and `gavel/recommendation` SARIF properties

### Optional: Run Gavel with a keyboard shortcut

Add a VS Code task so you can trigger Gavel analysis without leaving the editor. Create `.vscode/tasks.json` in your project:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Gavel: Analyze Project",
      "type": "shell",
      "command": "gavel",
      "args": ["analyze", "--dir", "."],
      "group": "build",
      "presentation": {
        "reveal": "silent",
        "panel": "shared"
      },
      "problemMatcher": []
    },
    {
      "label": "Gavel: Analyze Current File",
      "type": "shell",
      "command": "gavel",
      "args": ["analyze", "--files", "${file}"],
      "group": "build",
      "presentation": {
        "reveal": "silent",
        "panel": "shared"
      },
      "problemMatcher": []
    }
  ]
}
```

Bind a keyboard shortcut to either task:

1. Open **Keyboard Shortcuts** (`Ctrl+K Ctrl+S` / `Cmd+K Cmd+S`).
2. Search for **Tasks: Run Task**.
3. Assign your preferred shortcut.
4. When triggered, select **Gavel: Analyze Project** or **Gavel: Analyze Current File**.

After the task completes, open the new SARIF log via the Command Palette.

## Step 3b: Neovim

### Option A: SARIF to quickfix with jq

Convert SARIF findings to the standard `file:line:col: level: message` format that Neovim's `:cfile` understands.

The `jq` one-liner:

```sh
jq -r '
  .runs[0].results[] |
  "\(.locations[0].physicalLocation.artifactLocation.uri):\(.locations[0].physicalLocation.region.startLine):1: \(.level): \(.message.text)"
' .gavel/results/*/sarif.json
```

This produces output like:

```
src/handler.go:42:1: error: Unsanitized user input passed to exec.Command
src/db.go:18:1: warning: Database error ignored on line 18
```

Load that into Neovim's quickfix list:

```sh
jq -r '
  .runs[0].results[] |
  "\(.locations[0].physicalLocation.artifactLocation.uri):\(.locations[0].physicalLocation.region.startLine):1: \(.level): \(.message.text)"
' .gavel/results/*/sarif.json > /tmp/gavel-qf.txt
```

Then in Neovim:

```vim
:cfile /tmp/gavel-qf.txt
:copen
```

Use `:cnext` and `:cprev` to jump between findings.

### Shell alias for one-step workflow

Add this to your shell profile (`.bashrc`, `.zshrc`):

```sh
gavel-qf() {
  gavel analyze "$@"
  local sarif
  sarif=$(ls -t .gavel/results/*/sarif.json 2>/dev/null | head -1)
  if [ -z "$sarif" ]; then
    echo "No SARIF output found"
    return 1
  fi
  jq -r '
    .runs[0].results[] |
    "\(.locations[0].physicalLocation.artifactLocation.uri):\(.locations[0].physicalLocation.region.startLine):1: \(.level): \(.message.text)"
  ' "$sarif" > /tmp/gavel-qf.txt
  echo "Wrote $(wc -l < /tmp/gavel-qf.txt | tr -d ' ') findings to /tmp/gavel-qf.txt"
  echo "Open in Neovim: nvim -q /tmp/gavel-qf.txt"
}
```

Usage:

```sh
gavel-qf --dir ./src
nvim -q /tmp/gavel-qf.txt
```

### Option B: Use gavel review in a split terminal

Gavel ships with a built-in TUI for reviewing findings. Run it alongside Neovim in a terminal split or tmux pane:

```sh
gavel review --dir ./src
```

The TUI shows findings in a navigable list with full explanations, recommendations, and confidence scores. Use it as a companion to your editor -- review findings in the TUI, then switch to Neovim to make fixes.

### Future: Native LSP integration

A Gavel LSP server is planned that will deliver findings as native diagnostics in any LSP-capable editor, with no manual SARIF file handling. See the [LSP Integration Design](../plans/2026-02-05-lsp-integration-design.md) for details.

## Step 4: Iterate

The local workflow loop:

1. **Write** code in your editor.
2. **Analyze** with `gavel analyze --dir .` (or the VS Code task / `gavel-qf` alias).
3. **View** findings inline (SARIF Viewer / quickfix list).
4. **Fix** the flagged issues.
5. **Re-analyze** to confirm the fixes.

Each run creates a new timestamped directory under `.gavel/results/`. Old results are preserved so you can compare before and after.

## For Teams

**Shared configuration in the repo.** Commit `.gavel/policies.yaml` so every developer analyzes against the same policies. Local overrides in `~/.config/gavel/policies.yaml` let individuals adjust provider settings without changing the shared config.

**CI SARIF artifacts viewed locally.** If your CI workflow uploads SARIF as a build artifact (see the [CI/PR Gating Guide](./ci-pr-gating.md)), any team member can download the artifact and open it in VS Code with the SARIF Viewer -- same inline experience, no re-analysis needed.

**Consistent rules across environments.** Place custom rules in `.gavel/rules/` in the repository. Gavel ships 19 built-in rules and merges your custom rules on top. Everyone gets the same analysis regardless of their local setup.

## Tips

- **Use Ollama for fast local iteration.** A local `qwen2.5-coder:7b` model responds in seconds. Save cloud API calls for CI and final reviews.
- **Switch personas for focused review.** Use `--persona security` to get OWASP/CWE-focused findings, or `--persona architect` for design-level feedback. The default `code-reviewer` persona covers general quality.
- **Combine with the TUI.** Run `gavel review --dir .` to see findings in an interactive terminal UI with full explanations. Useful for triaging before you start fixing.
- **Analyze only changed files.** Use `--files` with specific paths instead of `--dir` to get faster results when you know what you changed.

## Next Steps

- [CI/PR Gating Guide](./ci-pr-gating.md) -- automate Gavel in your GitHub Actions workflow
- [README Configuration Reference](../../README.md#configuration) -- full policy format, provider options, and config merging
- [Custom Rules](../../README.md#custom-rules) -- write your own analysis rules with CWE/OWASP references
