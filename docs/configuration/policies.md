# Policies & Rules

## Policy Configuration

Gavel uses a tiered policy configuration system. Policies are merged in order of precedence (highest wins):

1. **Project** — `.gavel/policies.yaml`
2. **Machine** — `~/.config/gavel/policies.yaml`
3. **System defaults** — built into the binary

### Policy Format

```yaml
# Provider configuration (required)
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1

# Analysis policies
policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true

  function-length:
    description: "Functions should not exceed a reasonable length"
    severity: note
    instruction: "Flag functions longer than 50 lines."
    enabled: true

  my-custom-policy:
    description: "No hardcoded secrets"
    severity: error
    instruction: "Flag any hardcoded API keys, passwords, or tokens."
    enabled: true
```

### Default Policies

| Policy | Severity | Default | Description |
|--------|----------|---------|-------------|
| `shall-be-merged` | error | enabled | Catch-all quality gate — flags risky, sloppy, untested, or overly complex code |
| `function-length` | note | disabled | Flags functions longer than 50 lines |

### Merging Rules

- Non-empty string fields from a higher tier override lower tier values
- Setting `enabled: true` in a higher tier enables a policy
- Setting _only_ `enabled: false` (with no other fields) disables a policy from a lower tier

## Custom Rules

Gavel ships with 19 built-in analysis rules (15 regex + 4 AST) based on CWE, OWASP, and SonarQube standards. You can extend or override these with custom rule files.

### Rule Directories

Rules are loaded and merged in order of precedence (highest wins, by rule ID):

1. **Embedded defaults** — 19 rules built into the binary
2. **User rules** — `~/.config/gavel/rules/*.yaml` (personal rules for all projects)
3. **Project rules** — `.gavel/rules/*.yaml` (project-specific rules)

To use a different project rules directory for a single run:

```bash
gavel analyze --rules-dir ./my-rules --dir ./src
```

### Rule Format

```yaml
rules:
  - id: "CUSTOM-S001"
    name: "api-key-in-source"
    category: "security"        # security | reliability | maintainability
    pattern: '(?i)AKIA[0-9A-Z]{16}'
    languages: ["go", "python"] # optional — omit to match all languages
    level: "error"              # error | warning | note
    confidence: 0.95            # float in (0, 1]
    message: "Possible AWS access key committed to source"
    explanation: "..."
    remediation: "..."
    source: "Custom"            # CWE | OWASP | SonarQube | Custom
    cwe: ["CWE-798"]
    owasp: ["A07:2021"]
    references:
      - "https://cwe.mitre.org/data/definitions/798.html"
```
