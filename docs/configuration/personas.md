# Personas

Gavel supports different analysis personas for specialized expert perspectives. Personas work on both code and prose.

## Available Personas

| Persona | Focus | Best For |
|---------|-------|----------|
| `code-reviewer` (default) | Bugs, error handling, security, best practices | Daily PR reviews |
| `code-reviewer-verbose` | Same focus, more detailed prompt | Large models (Sonnet, GPT-4) |
| `architect` | Scalability, API design, service boundaries | Architecture reviews |
| `security` | OWASP Top 10, auth/authz, injection vulnerabilities | Security audits |
| `research-assistant` | Evidence gaps, weak arguments, logical leaps | Technical/persuasive writing |
| `sharp-editor` | Clarity, wordiness, passive voice, structure | Prose editing |

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
- **`research-assistant`** — Use when reviewing technical or persuasive writing. Finds claims lacking evidence, logical gaps, and areas that need deeper research. Curious and constructive tone.
- **`sharp-editor`** — Use when editing prose for clarity and impact. Cuts wordiness, flags passive voice and weak verbs, improves structure and flow. Direct and opinionated tone.
