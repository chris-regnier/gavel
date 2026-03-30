# Quick Start

By the end of this page you will have analyzed your own code with an AI reviewer and received a verdict on whether it should be merged.

## What You'll See

After running Gavel, you get a verdict like this:

```json
{
  "decision": "review",
  "reason": "Decision: review based on 3 findings",
  "relevant_findings": [
    {
      "ruleId": "S1086",
      "level": "warning",
      "message": {
        "text": "Error return value from database query is silently discarded"
      },
      "properties": {
        "gavel/confidence": 0.88,
        "gavel/explanation": "The error returned by db.Exec() is assigned to _ and never checked. If the query fails, the caller has no way to know.",
        "gavel/recommendation": "Assign the error to a named variable and return it or handle it explicitly."
      }
    }
  ]
}
```

- **`decision`** — merge (clean), reject (blocking issues), or review (needs human eyes)
- **`gavel/confidence`** — how certain the AI is (0.0 to 1.0)
- **`gavel/explanation`** — why this is a problem
- **`gavel/recommendation`** — how to fix it

Now let's set that up.

## 1. Install Gavel

Download the latest release:

```bash
# macOS (Apple Silicon)
curl -L https://github.com/chris-regnier/gavel/releases/latest/download/gavel_Darwin_arm64.tar.gz | tar xz
sudo mv gavel_Darwin_arm64 /usr/local/bin/gavel

# macOS (Intel)
curl -L https://github.com/chris-regnier/gavel/releases/latest/download/gavel_Darwin_x86_64.tar.gz | tar xz
sudo mv gavel_Darwin_x86_64 /usr/local/bin/gavel

# Linux (amd64)
curl -L https://github.com/chris-regnier/gavel/releases/latest/download/gavel_Linux_x86_64.tar.gz | tar xz
sudo mv gavel_Linux_x86_64 /usr/local/bin/gavel
```

Or build from source — see [Installation](installation.md).

## 2. Set Up a Provider

You need an LLM to power the analysis. Pick the path that works for you:

### Cloud (fastest start)

Sign up at [openrouter.ai](https://openrouter.ai) and grab an API key. Then:

```bash
export OPENROUTER_API_KEY=sk-or-...
```

That's it. One environment variable, no local setup.

Other cloud options: [Anthropic](PROVIDERS.md#anthropic-direct-api), [OpenAI](PROVIDERS.md#openai-cloud-api), [AWS Bedrock](PROVIDERS.md#aws-bedrock-enterprise). See [Providers](PROVIDERS.md) for all options.

### Local (free, private)

Install [Ollama](https://ollama.ai/) and pull a fast code model:

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a model and start the server
ollama pull qwen2.5-coder:7b
ollama serve &
```

No API key needed. Everything runs on your machine.

## 3. Configure Gavel

The fastest way to get a config tailored to your project is to generate one:

```bash
gavel create config "Go REST API with PostgreSQL, focus on security"
```

This creates `.gavel/policies.yaml` with a provider, persona, and starter policies matched to your description.

Alternatively, create the config manually:

**For OpenRouter:**

```yaml
# .gavel/policies.yaml
provider:
  name: openrouter
  openrouter:
    model: google/gemini-2.0-flash-exp

policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true
```

**For Ollama:**

```yaml
# .gavel/policies.yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1

policies:
  shall-be-merged:
    description: "Shall this code be merged?"
    severity: error
    instruction: "Flag code that is risky, sloppy, untested, or unnecessarily complex."
    enabled: true
```

See [Generating Configuration](guides/generating-config.md) for more on AI-generated configs, rules, and personas.

## 4. Analyze Your Code

```bash
# Analyze a directory
gavel analyze --dir ./src

# Or analyze specific files
gavel analyze --files main.go,handler.go

# Or analyze a diff (e.g., from a PR)
git diff main...HEAD | gavel analyze --diff -
```

Gavel prints an analysis summary and saves a SARIF file under `.gavel/results/`.

## 5. Judge the Results

```bash
gavel judge
```

This evaluates the findings against Rego policies and returns the verdict: merge, reject, or review.

### What just happened?

Gavel just reviewed your code the way a senior engineer would:

1. **Read** your source files (or diff)
2. **Analyzed** each one against your policies using an LLM — looking for real bugs, not just style issues
3. **Ran** 19 built-in rules instantly (regex + tree-sitter AST) for common security and reliability patterns
4. **Produced** structured findings in standard SARIF format with confidence scores, explanations, and fix recommendations
5. **Evaluated** those findings against gate policies to decide: is this code safe to merge?

## What Next?

Pick the path that matches what you want to do:

- **See findings in your editor** — [Editor Integration](guides/editor-integration.md) for VS Code and Neovim
- **Gate every PR automatically** — [CI/PR Gating Guide](guides/ci-pr-gating.md) for GitHub Actions
- **Focus on security** — [For Security Teams](guides/for-security-teams.md) to catch OWASP Top 10 issues
- **Customize what Gavel checks** — [Policies & Rules](configuration/policies.md) for custom policies and rules
- **Switch analysis perspective** — [Personas](configuration/personas.md) for code review, architecture, or security focus
- **Generate configs with AI** — [Generating Configuration](guides/generating-config.md) for policies, rules, and personas from natural language
