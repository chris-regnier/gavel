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

## Advanced Configuration

### Strict Filter

When `strict_filter` is enabled (the default), Gavel appends an applicability filter to the persona prompt that tells the LLM to only report findings directly relevant to the analyzed artifact. Set to `false` to allow broader observations:

```yaml
strict_filter: false  # default: true
```

### Additional Contexts

Policies can pull in additional context files during analysis using `additional_contexts`. This is useful when a policy needs to reference related files (e.g., interface definitions, configuration schemas):

```yaml
policies:
  api-consistency:
    description: "API responses should follow the shared schema"
    severity: warning
    instruction: "Check that responses match the OpenAPI spec"
    enabled: true
    additional_contexts:
      - pattern: "api/openapi.yaml"        # file to include as context
        only_for: ["api/**/*.go"]           # only when analyzing these files
```

### Remote Cache

Share analysis results across CI and local environments:

```yaml
remote_cache:
  enabled: true
  url: "https://gavel-cache.company.com"
  auth:
    type: bearer                # "bearer", "api_key", or empty for none
    token_file: /path/to/token  # or use `token:` for inline value
  strategy:
    write_to_remote: true       # upload results after analysis
    read_from_remote: true      # check remote before analyzing
    prefer_local: true          # prefer local cache over remote
    warm_local_on_remote_hit: true  # save remote hits to local cache
```

Cache keys are deterministic hashes of file content + policies + model + BAML templates, so results are shared when analysis inputs match regardless of environment.

### Telemetry

Gavel supports OpenTelemetry for distributed tracing:

```yaml
telemetry:
  enabled: true
  endpoint: "https://otel-collector.company.com:4317"
  protocol: grpc          # "grpc" or "http"
  insecure: false
  service_name: gavel
  service_version: "0.2.0"
  sample_rate: 1.0         # 0.0 to 1.0
  headers:
    Authorization: "Bearer <token>"
```

### Calibration

Online calibration adjusts analysis based on community feedback:

```yaml
calibration:
  enabled: true
  server_url: "https://calibration.gavel.dev"
  api_key_env: GAVEL_CALIBRATION_KEY  # env var containing API key
  share_code: false                    # whether to share code snippets
  retrieve:
    enabled: true
    include_examples: true
    top_k: 5
    timeout_ms: 2000
  upload:
    enabled: true
    include_implicit: false
    batch_size: 10
```
