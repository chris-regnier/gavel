# CLI Usage

## `analyze`

Analyze source code against enabled policies using an LLM.

```bash
# Analyze a directory
gavel analyze --dir ./src

# Analyze specific files
gavel analyze --files main.go,handler.go

# Analyze a diff (e.g., from a PR)
git diff main...HEAD | gavel analyze --diff -

# Analyze a diff file
gavel analyze --diff changes.patch
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--dir` | Directory to recursively scan | — |
| `--files` | Comma-separated list of files | — |
| `--diff` | Path to unified diff (`-` for stdin) | — |
| `--output` | Output directory for results | `.gavel/results` |
| `--policies` | Directory containing `policies.yaml` | `.gavel` |
| `--rules-dir` | Custom rules directory (overrides `.gavel/rules/`) | — |
| `--cache-server` | Remote cache server URL to upload results | — |

Only one of `--dir`, `--files`, or `--diff` may be specified.

### Output

Writes a SARIF file and prints a JSON summary to stdout:

```json
{
  "id": "2026-02-18T15-30-31Z-e3980f",
  "findings": 3,
  "scope": "directory",
  "persona": "code-reviewer",
  "suppressed": 1
}
```

The SARIF file is stored at `.gavel/results/<id>/sarif.json`.

## `judge`

Evaluate a SARIF log against Rego policies to produce a gating decision.

```bash
# Judge the most recent analysis
gavel judge

# Judge a specific analysis by ID
gavel judge --result 2026-02-18T15-30-31Z-e3980f
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--result` | Analysis result ID to evaluate | most recent |
| `--output` | Directory containing analysis results | `.gavel/results` |
| `--rego` | Rego policies directory | `.gavel/rego` |
| `--policies` | Directory containing `policies.yaml` | `.gavel` |

### Output

```json
{
  "decision": "review",
  "reason": "Decision: review based on 3 findings",
  "relevant_findings": [...]
}
```

## `review`

Launch an interactive terminal UI for reviewing findings from a previous analysis. By default loads the most recent analysis.

```bash
# Review the most recent analysis
gavel review

# Review a specific analysis
gavel review --result 2026-02-18T15-30-31Z-e3980f

# Review a SARIF file directly
gavel review path/to/sarif.json
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--result` | Analysis result ID to review | most recent |
| `--output` | Directory containing analysis results | `.gavel/results` |

### Arguments

| Argument | Description |
|----------|-------------|
| `[sarif-file]` | Optional path to a SARIF file to load directly |

## `suppress`

Suppress a finding rule so it is excluded from future analysis results and verdicts.

```bash
# Suppress a rule globally
gavel suppress RGX003 --reason "Acceptable in this project"

# Suppress a rule for a specific file
gavel suppress RGX003 --file src/legacy.go --reason "Legacy code, won't fix"
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--reason` | Reason for suppression (required) | — |
| `--file` | Restrict suppression to this file path | — (global) |

### Arguments

| Argument | Description |
|----------|-------------|
| `<rule-id>` | Rule ID to suppress (required) |

Suppressions are stored in `.gavel/suppressions.yaml`.

## `unsuppress`

Remove a previously added suppression.

```bash
# Remove a global suppression
gavel unsuppress RGX003

# Remove a file-specific suppression
gavel unsuppress RGX003 --file src/legacy.go
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--file` | Remove only the file-specific suppression | — (global) |

### Arguments

| Argument | Description |
|----------|-------------|
| `<rule-id>` | Rule ID to unsuppress (required) |

## `suppressions`

List active suppressions.

```bash
# List all suppressions
gavel suppressions

# Filter by source
gavel suppressions --source cli:
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--source` | Filter by source prefix (e.g., `cli:`, `mcp:`) | — |

### Output

```
RULE        FILE            SOURCE              REASON
RGX003      (all)           cli:user:alice      Acceptable in this project
AST002      src/legacy.go   cli:user:alice      Legacy code, won't fix
```

## `feedback`

Provide feedback on analysis findings to improve future analysis quality.

```bash
gavel feedback --result 2026-02-18T15-30-31Z-e3980f --finding 0 --verdict useful
gavel feedback --result 2026-02-18T15-30-31Z-e3980f --finding 2 --verdict noise --reason "False positive on test file"
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--result` | Analysis result ID (required) | — |
| `--finding` | Finding index, 0-based (required) | — |
| `--verdict` | `useful`, `noise`, or `wrong` (required) | — |
| `--reason` | Optional explanation for feedback | — |
| `--output` | Directory containing analysis results | `.gavel/results` |

### Output

```
Feedback recorded for result 2026-02-18T15-30-31Z-e3980f (finding #0: useful)
Total feedback: 3 (useful: 2, noise: 1, wrong: 0)
```

## `lsp`

Start gavel in LSP mode to provide real-time code analysis in your editor.

The LSP server listens on stdin/stdout and provides diagnostics as you edit files. See the [LSP Setup](../lsp-setup.md) guide for editor configuration.

```bash
gavel lsp
gavel lsp --cache-server https://gavel.company.com
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--machine-config` | Machine-level config file | `~/.config/gavel/policies.yaml` |
| `--project-config` | Project-level config file | `.gavel/policies.yaml` |
| `--cache-dir` | Cache directory | `~/.cache/gavel` |
| `--cache-server` | Remote cache server URL | — |

## `mcp`

Start gavel as an MCP (Model Context Protocol) server for AI agent integration.

The MCP server communicates over stdin/stdout, allowing AI assistants like Claude to analyze code, evaluate results, and manage suppressions programmatically.

```bash
gavel mcp
```

### Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--machine-config` | Machine-level config file | `~/.config/gavel/policies.yaml` |
| `--project-config` | Project-level config file | `.gavel/policies.yaml` |
| `--output` | Output directory for results | `.gavel/results` |
| `--rego-dir` | Directory containing custom Rego policies | embedded policy |

### Exposed capabilities

**Tools:** `analyze_file`, `analyze_directory`, `judge`, `list_results`, `get_result`, `suppress_finding`, `unsuppress_finding`, `list_suppressions`

**Resources:** `gavel://policies`, `gavel://results/{id}`

**Prompts:** `code-review`, `security-audit`, `architecture-review`

## `harness`

Run A/B experiments to compare analysis configurations.

See the [Harness](../harness.md) guide for detailed usage.

### `harness run`

```bash
gavel harness run variants.yaml
gavel harness run variants.yaml -n 5 -o results.jsonl
```

| Flag | Description | Default |
|------|-------------|---------|
| `-n`, `--runs` | Number of runs per variant | from config or 3 |
| `-o`, `--output` | Output JSONL file | `experiment-results-<timestamp>.jsonl` |
| `--packages` | Packages to analyze | from config |
| `--config` | Base config file path | `.gavel/policies.yaml` |

### `harness summarize`

```bash
gavel harness summarize results.jsonl
gavel harness summarize results.jsonl --baseline control -f json
```

| Flag | Description | Default |
|------|-------------|---------|
| `--baseline` | Baseline variant name for delta calculations | — |
| `-f`, `--format` | Output format: `text`, `json`, `yaml` | `text` |

## `create`

Generate configuration components from natural language descriptions using an LLM. Requires `OPENROUTER_API_KEY`.

See the [Generating Configuration](../guides/generating-config.md) guide for detailed usage and workflows.

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `create policy [description]` | Generate a policy from a description |
| `create rule [description]` | Generate a regex-based rule |
| `create persona [description]` | Generate a custom analysis persona |
| `create config [requirements]` | Generate a complete configuration |
| `create wizard` | Launch interactive TUI wizard |

### Examples

```bash
gavel create policy "Check that all public functions have doc comments"
gavel create rule --category=security "Detect hardcoded JWT secrets"
gavel create persona "A React hooks and performance expert"
gavel create config --provider=ollama "Go microservices with security focus"
gavel create wizard
```

### Flags

**`create policy`**, **`create persona`**:

| Flag | Description | Default |
|------|-------------|---------|
| `-o`, `--output` | Write to file instead of stdout | stdout |

**`create rule`**:

| Flag | Description | Default |
|------|-------------|---------|
| `-o`, `--output` | Write to file instead of stdout | stdout |
| `-c`, `--category` | `security`, `reliability`, or `maintainability` | `maintainability` |
| `-l`, `--languages` | Target languages (comma-separated) | `any` |

**`create config`**:

| Flag | Description | Default |
|------|-------------|---------|
| `-o`, `--output` | Output file path | `.gavel/policies.yaml` |
| `-p`, `--provider` | Preferred provider | auto-selected |

## `version`

Print version information.

```bash
gavel version
# gavel v0.2.0
#   commit: abc1234
#   built at: 2026-03-15T10:30:00Z
```

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--persona` | Persona for analysis (`code-reviewer`, `code-reviewer-verbose`, `architect`, `security`, `research-assistant`, `sharp-editor`) | `code-reviewer` |
| `-q`, `--quiet` | Suppress all log output | `false` |
| `-v`, `--verbose` | Enable verbose (info-level) logging | `false` |
| `--debug` | Enable debug-level logging | `false` |
