# For Developers

AI code review in your editor, before you push. Like having a senior engineer review your code instantly, for free, without waiting for PR feedback.

## The Local Dev Loop

```
Write code → gavel analyze → See findings inline → Fix → Repeat
```

Gavel runs locally, analyzes your code with an LLM, and delivers findings as structured output you can view in VS Code, Neovim, or the built-in TUI. Findings include confidence scores, explanations, and concrete fix recommendations — not just "error on line 42."

## Get Running in 2 Minutes

### Install Ollama (free, private, fast)

```bash
curl -fsSL https://ollama.ai/install.sh | sh
ollama pull qwen2.5-coder:7b
ollama serve &
```

### Generate a config for your project

```bash
gavel create config "Python Django app with REST API"
```

This creates `.gavel/policies.yaml` with provider settings and starter policies tailored to your description. Or write the config manually — see [Providers](../PROVIDERS.md).

### Analyze

```bash
gavel analyze --dir ./src
gavel judge
```

That's it. You now have structured findings with confidence scores, explanations, and fix recommendations.

## See Findings in Your Editor

### VS Code

Install the [SARIF Viewer extension](https://marketplace.visualstudio.com/items?itemName=MS-SarifVSCode.sarif-viewer), then open the SARIF log via Command Palette > **SARIF: Open SARIF Log** > `.gavel/results/<latest>/sarif.json`.

Findings appear as:
- Inline squiggles on affected lines
- Entries in the Problems panel
- Hover tooltips with explanations and recommendations

See the full [Editor Integration guide](editor-integration.md) for VS Code tasks and keyboard shortcuts.

### Neovim

Convert findings to quickfix format:

```bash
gavel-qf() {
  gavel analyze "$@"
  local sarif=$(ls -t .gavel/results/*/sarif.json 2>/dev/null | head -1)
  [ -z "$sarif" ] && echo "No SARIF output" && return 1
  jq -r '.runs[0].results[] |
    "\(.locations[0].physicalLocation.artifactLocation.uri):\(.locations[0].physicalLocation.region.startLine):1: \(.level): \(.message.text)"
  ' "$sarif" > /tmp/gavel-qf.txt
  echo "$(wc -l < /tmp/gavel-qf.txt | tr -d ' ') findings -> /tmp/gavel-qf.txt"
}
```

Then: `gavel-qf --dir ./src && nvim -q /tmp/gavel-qf.txt`

See the full [Editor Integration guide](editor-integration.md) for LSP setup and more options.

### Built-in TUI

Browse findings interactively without leaving the terminal:

```bash
gavel review
```

Navigate findings with full explanations, confidence scores, and recommendations.

## Generate Custom Configs with AI

Gavel's `create` commands generate policies, rules, and personas from natural language:

```bash
# Generate a policy
gavel create policy "Ensure all public functions have doc comments"

# Generate a security rule
gavel create rule --category=security "Detect hardcoded JWT secrets in Go code"

# Generate a custom persona
gavel create persona "A React hooks and performance expert"

# Launch the interactive wizard
gavel create wizard
```

See [Generating Configuration](generating-config.md) for the full guide.

## Switch Perspectives with Personas

Different personas give you different expert viewpoints:

```bash
# Default: general code review
gavel analyze --dir ./src

# Security-focused (OWASP Top 10, auth, injection)
gavel analyze --dir ./src --persona security

# Architecture-focused (scalability, API design, coupling)
gavel analyze --dir ./src --persona architect
```

Available personas: `code-reviewer` (default), `code-reviewer-verbose`, `architect`, `security`, `research-assistant`, `sharp-editor`. See [Personas](../configuration/personas.md).

## Manage Noise with Suppressions

Legacy code or intentional patterns can generate findings you want to ignore:

```bash
# Suppress a rule globally
gavel suppress S1135 --reason "We use TODO comments intentionally"

# Suppress a rule for one file
gavel suppress AST001 --file src/legacy.go --reason "Legacy, won't refactor"

# List and remove suppressions
gavel suppressions
gavel unsuppress S1135
```

Suppressed findings still appear in SARIF (for audit) but don't affect the verdict. See [Suppressions](suppressions.md).

## Use Gavel with AI Assistants (MCP)

Gavel includes an MCP server that lets AI assistants like Claude analyze code, review results, and manage suppressions programmatically:

```bash
gavel mcp
```

Add it to your Claude Code config to give Claude direct access to code analysis:

```json
{
  "mcpServers": {
    "gavel": {
      "command": "gavel",
      "args": ["mcp"]
    }
  }
}
```

The MCP server exposes tools for `analyze_file`, `analyze_directory`, `judge`, `list_results`, `get_result`, `suppress_finding`, `unsuppress_finding`, and `list_suppressions`.

## Tips

- **Analyze only what changed.** Use `--files main.go,handler.go` instead of `--dir .` for faster results when you know what you touched.
- **Use Ollama for iteration.** Local models respond in 1-3 seconds per file. Save cloud API calls for CI.
- **Combine input modes.** `git diff HEAD~1 | gavel analyze --diff -` reviews only your latest changes.
- **Provide feedback.** `gavel feedback --result <id> --finding 0 --verdict noise` helps calibrate future analysis.

## Next Steps

- **[Gate every PR](ci-pr-gating.md)** — automate Gavel in GitHub Actions
- **[Policies & Rules](../configuration/policies.md)** — customize what Gavel checks for
- **[Try on Open Source](try-on-open-source.md)** — see Gavel on real Go, Python, and TypeScript repos
