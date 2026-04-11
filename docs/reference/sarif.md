# SARIF Extensions

Gavel produces standard [SARIF 2.1.0](https://sarifweb.azurewebsites.net/) output with additional properties under the `gavel/` namespace.

## Run-Level Properties

These properties appear on each SARIF run (in `runs[0].properties`):

| Property | Type | Description |
|----------|------|-------------|
| `gavel/inputScope` | string | Input type: `files`, `diff`, or `directory` |
| `gavel/persona` | string | Persona used for analysis (e.g., `code-reviewer`) |

## Taxonomies

Rules that reference CWE or OWASP categories emit standard SARIF taxonomies in `runs[0].taxonomies` and `reportingDescriptor.relationships`. This enables interoperability with GitHub Advanced Security, Semgrep, Snyk, DefectDojo, and other SARIF-aware security dashboards.

```json
{
  "taxonomies": [{
    "name": "CWE",
    "organization": "MITRE",
    "taxa": [
      { "id": "798" },
      { "id": "89" }
    ]
  }],
  "tool": {
    "driver": {
      "rules": [{
        "id": "S2068",
        "relationships": [{
          "target": { "id": "798", "toolComponent": { "name": "CWE" } },
          "kinds": ["relevant"]
        }]
      }]
    }
  }
}
```

Taxonomies are built automatically from the `cwe` and `owasp` fields in rule definitions. No additional configuration is needed.

## Result-Level Properties

These properties appear on each finding (in `results[].properties`):

### Core properties (all findings)

| Property | Type | Description |
|----------|------|-------------|
| `gavel/confidence` | float (0.0-1.0) | Confidence in the finding |
| `gavel/explanation` | string | Detailed reasoning behind the finding |
| `gavel/tier` | string | Analysis tier: `instant`, `fast`, or `comprehensive` |

### LLM findings (fast/comprehensive tier)

| Property | Type | Description |
|----------|------|-------------|
| `gavel/recommendation` | string | Suggested fix or action |
| `gavel/cache_key` | string | Deterministic hash of analysis inputs (file content + policies + model + BAML templates) |
| `gavel/analyzer` | object | Provider/model metadata (`provider`, `model`, `policies`) |

### Instant-tier findings (regex and AST rules)

| Property | Type | Description |
|----------|------|-------------|
| `gavel/rule-source` | string | Rule origin: `CWE`, `OWASP`, `SonarQube`, or `Custom` |
| `gavel/rule-type` | string | `ast` for tree-sitter checks (absent for regex) |
| `gavel/remediation` | string | Remediation guidance |
| `gavel/references` | string[] | External reference URLs |

## Example Finding

```json
{
  "ruleId": "shall-be-merged",
  "level": "error",
  "message": {
    "text": "Error from cmd.Execute() is silently discarded"
  },
  "locations": [{
    "physicalLocation": {
      "artifactLocation": { "uri": "main.go" },
      "region": { "startLine": 10, "endLine": 12 }
    }
  }],
  "properties": {
    "gavel/confidence": 0.9,
    "gavel/explanation": "The main function catches the error from Execute but discards it...",
    "gavel/recommendation": "Log the error and exit with a non-zero status code",
    "gavel/tier": "comprehensive",
    "gavel/cache_key": "a1b2c3d4e5f6...",
    "gavel/analyzer": {
      "provider": "anthropic",
      "model": "claude-haiku-4-5",
      "policies": ["shall-be-merged"]
    }
  }
}
```

## Suppressed Findings

Suppressed findings include a standard SARIF `suppressions` array:

```json
{
  "ruleId": "RGX003",
  "suppressions": [{
    "kind": "external",
    "justification": "Acceptable in this project",
    "properties": {
      "gavel/source": "cli:user:alice",
      "gavel/created": "2026-03-29T14:30:45Z"
    }
  }]
}
```

See [Suppressing Findings](../guides/suppressions.md) for details.

## Compatibility

SARIF output is compatible with:

- **GitHub Code Scanning** -- upload via the `github/codeql-action/upload-sarif` action
- **VS Code SARIF Viewer** -- view findings inline in the editor
- **Any SARIF-compatible tool** -- standard 2.1.0 format with extensions in `properties`
