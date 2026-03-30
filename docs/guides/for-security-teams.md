# For Security Teams

Catch OWASP Top 10 issues in every pull request. Unlike SAST tools that only match patterns, Gavel understands context — it knows when user input flows into a SQL query, not just that a query exists.

## What Gavel Catches

Gavel combines two tiers of analysis:

**Instant tier (no LLM, milliseconds):** 15 regex rules and 4 tree-sitter AST checks run on every file. These catch structural patterns like hardcoded credentials, SQL injection via string concatenation, and command injection.

**LLM tier (seconds per file):** An AI model analyzes code against your policies, finding context-dependent issues that pattern matching misses — data flow problems, missing authorization checks, insecure configurations.

### Security Findings Examples

**SQL injection (S3649):**

```json
{
  "ruleId": "S3649",
  "level": "error",
  "message": {
    "text": "SQL injection: raw user input interpolated into query via fmt.Sprintf"
  },
  "properties": {
    "gavel/confidence": 0.95,
    "gavel/explanation": "The function UnsafeQueryGetData builds a SQL query using fmt.Sprintf with user-supplied uid directly interpolated into the query string.",
    "gavel/recommendation": "Use parameterized queries with placeholder arguments instead of string interpolation."
  }
}
```

**Hardcoded secrets (S2068):**

```json
{
  "ruleId": "S2068",
  "level": "error",
  "message": {
    "text": "Hardcoded secret: Django SECRET_KEY is set to a static string in source code"
  },
  "properties": {
    "gavel/confidence": 0.97,
    "gavel/explanation": "The SECRET_KEY setting is hardcoded as a string literal. This key is used for cryptographic signing and must not be committed to version control.",
    "gavel/recommendation": "Load SECRET_KEY from an environment variable or a secrets manager."
  }
}
```

### Built-in Security Rules

| ID | Name | Level | What It Catches |
|----|------|-------|-----------------|
| S2068 | hardcoded-credentials | error | Hard-coded passwords, API keys, tokens |
| S3649 | sql-injection | error | SQL injection via string concatenation |
| S2076 | command-injection | error | OS command injection from user input |
| S2083 | path-traversal | warning | File path traversal with user input |
| S4426 | weak-crypto | warning | Use of MD5, SHA1, DES, or RC4 |
| S4830 | insecure-tls | error | TLS certificate verification disabled |

All rules include CWE and OWASP references for compliance reporting.

## Set Up Security-Focused Analysis

### Use the security persona

The `security` persona shifts the AI's focus to OWASP Top 10, authentication, authorization, and injection vulnerabilities:

```bash
gavel analyze --dir ./src --persona security
```

Or set it in your config:

```yaml
# .gavel/policies.yaml
persona: security

provider:
  name: anthropic
  anthropic:
    model: claude-haiku-4-5

policies:
  shall-be-merged:
    description: "Security gate"
    severity: error
    instruction: "Flag security vulnerabilities: injection, broken auth, sensitive data exposure, XXE, broken access control, misconfigurations, XSS, insecure deserialization, vulnerable components, insufficient logging."
    enabled: true
```

### Add custom security rules

Create `.gavel/rules/security.yaml`:

```yaml
rules:
  - id: "CUSTOM-S001"
    name: "aws-key-in-source"
    category: "security"
    pattern: '(?i)AKIA[0-9A-Z]{16}'
    level: "error"
    confidence: 0.95
    message: "Possible AWS access key committed to source"
    explanation: "AWS access keys should never be committed to version control."
    remediation: "Use environment variables or AWS IAM roles."
    source: "Custom"
    cwe: ["CWE-798"]
    owasp: ["A07:2021"]

  - id: "CUSTOM-S002"
    name: "jwt-secret-in-source"
    category: "security"
    pattern: '(?i)(jwt[_-]?secret|signing[_-]?key)\s*[:=]\s*["\x27][^"\x27]{8,}'
    level: "error"
    confidence: 0.90
    message: "Possible JWT signing secret hardcoded in source"
    explanation: "JWT signing secrets should be loaded from environment or a secrets manager."
    remediation: "Move the secret to an environment variable."
    source: "Custom"
    cwe: ["CWE-798"]
    owasp: ["A02:2021"]
```

Or generate one with AI:

```bash
gavel create rule --category=security "Detect hardcoded JWT secrets in Go and Python"
```

### Tune the gate threshold

The default Rego policy rejects when any error-level finding has confidence above 0.8. For security-focused gating, you might want to be more aggressive. Create `.gavel/rego/security.rego`:

```rego
package gavel.gate

import rego.v1

default decision := "review"

# Filter out suppressed findings
_suppressed(result) if {
    suppressions := object.get(result, "suppressions", [])
    count(suppressions) > 0
}

unsuppressed_results contains result if {
    some result in input.runs[0].results
    not _suppressed(result)
}

# Reject on any unsuppressed security finding (error or warning)
decision := "reject" if {
    some result in unsuppressed_results
    result.level in {"error", "warning"}
    result.properties["gavel/confidence"] > 0.7
}

# Auto-merge only if no findings at all
decision := "merge" if {
    count(unsuppressed_results) == 0
}
```

This is stricter than the default: it rejects on warnings too, and uses a 0.7 confidence threshold instead of 0.8.

## CI Integration for Security

Add the `--persona security` flag to your GitHub Actions workflow:

```yaml
      - name: Run Gavel analysis
        if: env.SKIP_ANALYSIS != 'true'
        env:
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
        run: |
          /tmp/gavel analyze \
            --diff /tmp/pr.diff \
            --policies .github \
            --output /tmp/gavel-results \
            --persona security
```

See the full [CI/CD guide](for-ci-cd.md) for the complete workflow setup.

### SARIF and compliance

Gavel's SARIF output includes CWE and OWASP references for every rule-based finding. The SARIF file at `.gavel/results/<id>/sarif.json` can be:

- Uploaded to GitHub Code Scanning for native annotations
- Imported into any SARIF-compatible dashboard (DefectDojo, OWASP Dependency-Track)
- Archived for audit trails

## Suppress Known Exceptions

Some security findings may be intentional (e.g., test fixtures with hardcoded credentials):

```bash
# Suppress hardcoded-credentials in test files
gavel suppress S2068 --file tests/fixtures/auth.go --reason "Test fixture, not real credentials"

# List all suppressions
gavel suppressions
```

Suppressed findings stay in SARIF for audit but don't affect the verdict. See [Suppressions](suppressions.md).

## Next Steps

- **[Policies & Rules](../configuration/policies.md)** — full rule format with CWE/OWASP references
- **[Custom Rego](../configuration/rego.md)** — fine-tune gate logic
- **[CI/CD Guide](for-ci-cd.md)** — full GitHub Actions setup
- **[Try on Open Source](try-on-open-source.md)** — run Gavel on deliberately vulnerable repos (govwa, pygoat)
