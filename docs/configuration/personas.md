# Personas

Gavel supports different analysis personas for specialized code review. Each persona provides a different expert perspective.

## Available Personas

| Persona | Focus | Best For |
|---------|-------|----------|
| `code-reviewer` (default) | Bugs, error handling, security, best practices | Daily PR reviews |
| `code-reviewer-verbose` | Same focus, more detailed prompt | Large models (Sonnet, GPT-4) |
| `architect` | Scalability, API design, service boundaries | Architecture reviews |
| `security` | OWASP Top 10, auth/authz, injection vulnerabilities | Security audits |

## Usage

### Via Config

Set the persona in `.gavel/policies.yaml`:

```yaml
persona: security
```

### Via CLI Flag

```bash
gavel analyze --persona architect --dir ./src
```

The CLI flag overrides the config file.

## Choosing a Persona

- **`code-reviewer`** — The default. Optimized for small/fast models with a minimal ~50 word prompt. Good for daily PR review with Ollama or Haiku.
- **`code-reviewer-verbose`** — The original ~250 word detailed prompt. Better for large models (Sonnet, GPT-4, Opus) that follow complex instructions well.
- **`architect`** — Use when reviewing system design, API contracts, or service boundaries. Focuses on scalability and coupling rather than line-level bugs.
- **`security`** — Use before releases or for security-focused audits. Focuses on OWASP Top 10, authentication, authorization, and injection vulnerabilities.
