# Suppressing Findings

Gavel lets you suppress specific rules so they are excluded from gating decisions. Suppressed findings still appear in SARIF output (marked with suppression metadata) but do not affect the `judge` verdict.

## Quick start

```bash
# Suppress a rule globally
gavel suppress RGX003 --reason "Acceptable in this project"

# Suppress a rule for a specific file
gavel suppress AST001 --file src/legacy.go --reason "Legacy code, won't fix"

# List active suppressions
gavel suppressions

# Remove a suppression
gavel unsuppress RGX003
```

## How suppressions work

1. **During `analyze`**: All rules run normally. Findings that match a suppression are marked in the SARIF output with a `suppressions` array but are **not removed**. The analyze summary reports a `suppressed` count.
2. **During `judge`**: The default Rego policy filters out suppressed findings. Only unsuppressed findings affect the gating decision. If all findings are suppressed, the verdict is `merge`.

This means you can always see what was suppressed by inspecting the SARIF file, even though suppressed findings don't block your CI pipeline.

## Suppression scope

| Scope | Command | Effect |
|-------|---------|--------|
| Global | `gavel suppress RGX003 --reason "..."` | Suppresses rule RGX003 in all files |
| File-specific | `gavel suppress RGX003 --file src/legacy.go --reason "..."` | Suppresses rule RGX003 only in `src/legacy.go` |

File-specific and global suppressions for the same rule are independent -- removing one does not affect the other.

## The suppressions file

Suppressions are stored in `.gavel/suppressions.yaml`:

```yaml
suppressions:
  - rule_id: RGX003
    reason: "Acceptable in this project"
    created: "2026-03-29T14:30:45Z"
    source: "cli:user:alice"
  - rule_id: AST001
    file: "src/legacy.go"
    reason: "Legacy code, won't fix"
    created: "2026-03-29T14:35:12Z"
    source: "cli:user:alice"
```

You can edit this file directly, but using the CLI commands is recommended to ensure correct formatting and timestamps.

## Filtering suppressions

List suppressions filtered by source:

```bash
# Show only CLI-created suppressions
gavel suppressions --source cli:

# Show only MCP/agent-created suppressions
gavel suppressions --source mcp:
```

The source field tracks who created the suppression:
- `cli:user:<username>` -- created via the CLI by a user
- `mcp:agent:<name>` -- created via the MCP server by an AI agent

## Removing suppressions

```bash
# Remove a global suppression
gavel unsuppress RGX003

# Remove a file-specific suppression only
gavel unsuppress AST001 --file src/legacy.go
```

## Suppressions in CI

Suppressions apply automatically in CI since they are stored in `.gavel/suppressions.yaml` (committed to your repository). When a team member suppresses a noisy rule locally and commits the file, CI runs will also respect that suppression.

## Custom Rego and suppressions

If you write custom Rego policies, you should account for suppressions. The default Rego policy filters suppressed findings like this:

```rego
_suppressed(result) if {
    suppressions := object.get(result, "suppressions", [])
    count(suppressions) > 0
}

unsuppressed_results contains result if {
    some result in input.runs[0].results
    not _suppressed(result)
}
```

Use `unsuppressed_results` instead of `input.runs[0].results` in your decision logic to respect suppressions.
