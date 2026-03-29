# Generating Configuration with AI

Gavel can generate policies, rules, personas, and complete configurations from natural language descriptions using an LLM. This is useful for bootstrapping a new project or creating custom analysis components without writing YAML by hand.

## Prerequisites

All generation commands require the `OPENROUTER_API_KEY` environment variable:

```bash
export OPENROUTER_API_KEY=sk-or-...
```

## Quick Start

```bash
# Generate a complete config for your project
gavel create config "Go microservices with PostgreSQL, focus on security and error handling"

# Generate a single policy
gavel create policy "Check that all public functions have documentation comments"

# Generate a regex-based rule
gavel create rule --category=security "Detect hardcoded JWT secrets in Go code"

# Generate a custom persona
gavel create persona "A React expert who focuses on hooks and performance"

# Launch the interactive wizard
gavel create wizard
```

## Commands

### `create policy`

Generates a policy from a natural language description. Policies are high-level instructions that tell the LLM what to look for during analysis.

```bash
gavel create policy "Ensure all error returns include context wrapping"
```

Output (YAML):

```yaml
wrap_error_returns:
  description: Check that error returns wrap context with fmt.Errorf or errors.Wrap
  severity: warning
  instruction: >-
    Look for functions that return errors without adding context. Every error
    return should wrap the original error with additional context about what
    operation failed, using fmt.Errorf with %w or a wrapping library.
  enabled: true
```

| Flag | Description | Default |
|------|-------------|---------|
| `-o`, `--output` | Write to file instead of stdout | stdout |

### `create rule`

Generates a regex-based rule with CWE/OWASP references, confidence scores, and remediation guidance.

```bash
gavel create rule --category=security --languages=go,python \
  "Detect SQL queries built with string concatenation"
```

| Flag | Description | Default |
|------|-------------|---------|
| `-o`, `--output` | Write to file instead of stdout | stdout |
| `-c`, `--category` | Rule category: `security`, `reliability`, `maintainability` | `maintainability` |
| `-l`, `--languages` | Target languages (comma-separated) | `any` |

Generated rules follow the ID convention `CUSTOM-S001` (security), `CUSTOM-R001` (reliability), `CUSTOM-M001` (maintainability).

### `create persona`

Generates a custom analysis persona with a complete system prompt, including role definition, focus areas, tone, and confidence guidance.

```bash
gavel create persona "A database expert who focuses on query performance and ORM anti-patterns"
```

Output (YAML):

```yaml
name: database-expert
display_name: Database Expert
system_prompt: |
  You are a database performance specialist with deep expertise in SQL optimization,
  ORM usage patterns, and data access layer design...

  FOCUS AREAS:
  - N+1 query patterns and eager loading opportunities
  - Missing indexes on frequently queried columns
  ...
```

| Flag | Description | Default |
|------|-------------|---------|
| `-o`, `--output` | Write to file instead of stdout | stdout |

The generator extracts focus areas from your description automatically. Keywords like "React", "performance", "security", "API", "database", "testing", "Go", and "Python" guide the focus areas.

### `create config`

Generates a complete `.gavel/policies.yaml` with provider settings, a persona, and starter policies tailored to your project.

```bash
gavel create config --provider=ollama \
  "I want to analyze a Python Django app for security and code quality"
```

| Flag | Description | Default |
|------|-------------|---------|
| `-o`, `--output` | Output file path | `.gavel/policies.yaml` |
| `-p`, `--provider` | Preferred provider: `ollama`, `openrouter`, `anthropic`, `bedrock`, `openai` | auto-selected |

If you don't specify a provider, the LLM picks one based on your requirements. The generated config includes 2-4 relevant starter policies.

### `create wizard`

Launches an interactive TUI for guided configuration generation. The wizard walks you through creating policies, rules, personas, or a full config with a menu-driven interface.

```bash
gavel create wizard
```

Navigation:
- **Arrow keys** to select a menu item
- **Enter** to confirm
- **Esc** to go back
- **Ctrl+C** or **q** to quit

## Workflows

### Bootstrap a new project

```bash
# Generate a tailored config
gavel create config "Go REST API with JWT auth, PostgreSQL, and Docker" \
  -o .gavel/policies.yaml

# Review and adjust the generated config
cat .gavel/policies.yaml

# Run your first analysis
gavel analyze --dir ./src
```

### Add a custom rule to an existing project

```bash
# Generate the rule
gavel create rule --category=security \
  "Detect use of md5 or sha1 for password hashing" \
  -o .gavel/rules/custom.yaml

# The rule is now active on the next analysis
gavel analyze --dir ./src
```

### Create a team-specific persona

```bash
# Generate and review the persona
gavel create persona "A frontend accessibility expert who checks for WCAG compliance"

# Save it once you're happy with the output
gavel create persona "A frontend accessibility expert who checks for WCAG compliance" \
  -o .gavel/personas.yaml
```

## How It Works

The `create` commands use BAML-defined generation functions that call an LLM (via OpenRouter) to produce structured output. The LLM receives your natural language description along with instructions about the expected output format (policy fields, rule conventions, persona structure, or full config layout). The structured response is then converted to YAML.

Generation is separate from analysis — it uses the OpenRouter provider regardless of what provider your project is configured to use for `analyze`.
