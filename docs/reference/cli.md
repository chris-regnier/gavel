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
| `--policies` | Policy config directory | `.gavel` |
| `--rules-dir` | Custom rules directory (overrides `.gavel/rules/`) | — |
| `--cache-server` | Remote cache server URL to upload results | — |

### Output

Writes a SARIF file and prints a JSON summary to stdout:

```json
{
  "id": "2026-02-18T15-30-31Z-e3980f",
  "findings": 3,
  "scope": "directory",
  "persona": "code-reviewer"
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
| `--policies` | Policy config directory | `.gavel` |

### Output

```json
{
  "decision": "review",
  "reason": "Decision: review based on 3 findings",
  "relevant_findings": [...]
}
```

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--persona` | Persona for analysis (`code-reviewer`, `architect`, `security`) | `code-reviewer` |
| `-q`, `--quiet` | Suppress all log output | `false` |
| `-v`, `--verbose` | Enable verbose (info-level) logging | `false` |
| `--debug` | Enable debug-level logging | `false` |
