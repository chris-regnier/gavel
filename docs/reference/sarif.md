# SARIF Extensions

Gavel produces standard [SARIF 2.1.0](https://sarifweb.azurewebsites.net/) output with additional properties under the `gavel/` namespace.

## Extension Properties

| Property | Type | Description |
|----------|------|-------------|
| `gavel/confidence` | float (0.0–1.0) | LLM confidence in the finding |
| `gavel/explanation` | string | Detailed reasoning behind the finding |
| `gavel/recommendation` | string | Suggested fix or action |
| `gavel/inputScope` | string | Input type: `files`, `diff`, or `directory` |

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
    "gavel/recommendation": "Log the error and exit with a non-zero status code"
  }
}
```

## Compatibility

SARIF output is compatible with:

- **GitHub Code Scanning** — upload via the `github/codeql-action/upload-sarif` action
- **VS Code SARIF Viewer** — view findings inline in the editor
- **Any SARIF-compatible tool** — standard 2.1.0 format with extensions in `properties`
