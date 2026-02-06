# Provider Configuration Guide

Gavel supports multiple LLM providers to give you flexibility in choosing the best option for your needs based on cost, speed, quality, and deployment environment.

## Quick Start

1. Choose a provider from the table below
2. Set up required credentials (API keys or AWS credentials)
3. Create or update `.gavel/policies.yaml` with your provider configuration
4. Run `gavel analyze` - it will use your configured provider automatically

## Supported Providers

| Provider | Type | Speed | Cost | Quality | Best For |
|----------|------|-------|------|---------|----------|
| **Ollama** | Local | ⚡⚡⚡ Fast | 💰 Free | ⭐⭐⭐ Good | Local development, privacy-sensitive work |
| **OpenRouter** | Cloud API | ⚡⚡ Variable | 💰💰 Variable | ⭐⭐⭐⭐ Excellent | Easy access to many models, experimentation |
| **Anthropic** | Cloud API | ⚡⚡ Fast | 💰💰💰 Premium | ⭐⭐⭐⭐⭐ Excellent | Production workloads, highest quality |
| **Bedrock** | AWS Cloud | ⚡⚡ Fast | 💰💰💰 Premium | ⭐⭐⭐⭐⭐ Excellent | Enterprise AWS environments |
| **OpenAI** | Cloud API | ⚡⚡⚡ Fast | 💰💰 Moderate | ⭐⭐⭐⭐ Very Good | General purpose, GPT-4 users |

## Configuration Examples

### Ollama (Local, Free)

**Setup:**
```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a fast code model
ollama pull qwen2.5-coder:7b
# Or pull a larger, more capable model
ollama pull gpt-oss:20b
```

**Config (`.gavel/policies.yaml`):**
```yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b  # Fast and accurate
    base_url: http://localhost:11434/v1
```

**Fast models:**
- `qwen2.5-coder:7b` - Very fast, excellent for code (recommended)
- `deepseek-coder-v2:16b` - Balanced speed/quality
- `codestral:22b` - High quality, slower

### OpenRouter (Cloud, Pay-as-you-go)

**Setup:**
```bash
# Get API key from https://openrouter.ai/keys
export OPENROUTER_API_KEY=sk-or-...
```

**Config (`.gavel/policies.yaml`):**
```yaml
provider:
  name: openrouter
  openrouter:
    model: google/gemini-2.0-flash-001  # Very fast and cheap
```

**Recommended models:**
- `google/gemini-2.0-flash-exp` - Very fast, excellent value (~$0.10/$0.30 per 1M tokens)
- `anthropic/claude-3-5-haiku-20241022` - Fast Claude Haiku 3.5, good quality (~$0.80/$4.00 per 1M tokens)
- `deepseek/deepseek-chat` - Very cheap, surprisingly good (~$0.14/$0.28 per 1M tokens)
- `anthropic/claude-3-5-sonnet-20241022` - High quality Sonnet (~$3.00/$15.00 per 1M tokens)

### Anthropic (Direct API)

**Setup:**
```bash
# Get API key from https://console.anthropic.com/
export ANTHROPIC_API_KEY=sk-ant-...
```

**Config (`.gavel/policies.yaml`):**
```yaml
provider:
  name: anthropic
  anthropic:
    model: claude-3-5-haiku-20241022  # Fast, cost-effective (recommended)
```

**Available models:**
- `claude-3-5-haiku-20241022` - Fast Haiku 3.5, cost-effective (~$0.80/$4.00 per 1M tokens) **(recommended)**
- `claude-3-5-sonnet-20241022` - High quality Sonnet 3.5 (~$3.00/$15.00 per 1M tokens)
- `claude-opus-4-6-20260205` - Highest quality Opus 4.6, released Feb 5, 2026 (~$15.00/$75.00 per 1M tokens)

### AWS Bedrock (Enterprise)

**Setup:**
```bash
# Configure AWS credentials (one of these methods):
aws configure  # Interactive setup
# OR set environment variables
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1
```

**Config (`.gavel/policies.yaml`):**
```yaml
provider:
  name: bedrock
  bedrock:
    model: anthropic.claude-3-5-haiku-20241022-v1:0
    region: us-east-1
```

**Available models (by region):**
- `anthropic.claude-3-5-haiku-20241022-v1:0` - Fast Haiku 3.5, cost-effective **(recommended)**
- `global.anthropic.claude-sonnet-4-5-20250929-v1:0` - Sonnet 4.5 with global endpoint
- `anthropic.claude-opus-4-6-20260205-v1:0` - Opus 4.6 (highest quality, released Feb 5, 2026)

**Note:** Model availability varies by AWS region. Check the AWS Bedrock console for your region.

### OpenAI (Cloud API)

**Setup:**
```bash
# Get API key from https://platform.openai.com/api-keys
export OPENAI_API_KEY=sk-proj-...
```

**Config (`.gavel/policies.yaml`):**
```yaml
provider:
  name: openai
  openai:
    model: gpt-5.3-codex  # Latest coding model (recommended)
```

**Recommended models:**
- `gpt-5.3-codex` - Latest coding-specialized model, excellent for code analysis **(recommended)**
- `gpt-5.2` - Newest flagship general model (~$2.50/$10.00 per 1M tokens)
- `o3-mini` - Fast reasoning model for math/science/coding (~$0.15/$0.60 per 1M tokens)

## Speed Comparison

For fastest analysis times (approximate, varies by code complexity):

1. **Ollama** with `qwen2.5-coder:7b` - ~1-3 seconds per file (local)
2. **OpenRouter** with `google/gemini-2.0-flash-exp` - ~2-5 seconds per file
3. **Anthropic** with `claude-3-5-haiku-20241022` - ~3-6 seconds per file
4. **OpenAI** with `o3-mini` - ~2-5 seconds per file
5. **Bedrock** with Haiku 3.5 - ~3-6 seconds per file

Flagship models (Sonnet 4.5, Opus 4.6, GPT-5.3-Codex) typically take 5-15 seconds per file but provide higher quality analysis.

## Cost Comparison

Approximate costs for analyzing 100 files (~50KB each, ~500K tokens total):

| Provider | Model | Input Cost | Output Cost | Total Est. |
|----------|-------|------------|-------------|------------|
| Ollama | Any | $0 | $0 | **$0** (free) |
| OpenRouter | deepseek/deepseek-chat | $0.07 | $0.14 | **$0.21** |
| OpenRouter | google/gemini-flash-exp | $0.05 | $0.15 | **$0.20** |
| OpenAI | o3-mini | $0.08 | $0.30 | **$0.38** |
| Anthropic | claude-3-5-haiku | $0.40 | $2.00 | **$2.40** |
| OpenAI | gpt-5.2 | $1.25 | $5.00 | **$6.25** |
| OpenAI | gpt-5.3-codex | $1.25 | $5.00 | **$6.25** |
| Anthropic | claude-3-5-sonnet | $1.50 | $7.50 | **$9.00** |
| Anthropic | claude-opus-4-6 | $7.50 | $37.50 | **$45.00** |

## Quality vs Speed vs Cost

```
Quality Priority (best results):
1. Anthropic Claude Opus 4.6 (released Feb 5, 2026)
2. Anthropic Claude Sonnet 4.5
3. OpenAI GPT-5.3-Codex (best for code analysis)
4. OpenAI GPT-5.2
5. Anthropic Claude 3.5 Haiku

Speed Priority (fastest analysis):
1. Ollama qwen2.5-coder:7b (local)
2. OpenRouter Gemini 2.0 Flash
3. OpenAI o3-mini
4. Anthropic Claude 3.5 Haiku
5. Standard models

Cost Priority (lowest cost):
1. Ollama (free, local)
2. OpenRouter DeepSeek (~$0.20 per 100 files)
3. OpenRouter Gemini Flash (~$0.20 per 100 files)
4. OpenAI o3-mini (~$0.40 per 100 files)
5. Anthropic Haiku 3.5 (~$2.40 per 100 files)
```

## Recommended Configurations

### Development (Fast Iteration)
```yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
    base_url: http://localhost:11434/v1
```
- Free, fast, good quality
- Perfect for rapid development cycles
- No API keys needed

### CI/CD (Speed + Quality Balance)
```yaml
provider:
  name: openrouter
  openrouter:
    model: google/gemini-2.0-flash-001
```
- Very fast, low cost
- Good quality for automated checks
- Easy to set up in CI

### Production (Highest Quality)
```yaml
provider:
  name: anthropic
  anthropic:
    model: claude-3-5-haiku-20241022
```
- Excellent code understanding
- Reliable, consistent results
- Worth the premium for critical reviews

### Enterprise (AWS Integration)
```yaml
provider:
  name: bedrock
  bedrock:
    model: anthropic.claude-3-5-haiku-20241022-v1:0
    region: us-east-1
```
- AWS-native integration
- Enterprise compliance
- Unified billing with AWS

## Switching Providers

You can easily switch providers by updating `.gavel/policies.yaml`. All providers use the same analysis logic and produce compatible SARIF output.

Example workflow:
1. Start with **Ollama** for free local development
2. Use **OpenRouter Gemini Flash** in CI for speed
3. Switch to **Anthropic Sonnet** for final PR reviews
4. Deploy with **Bedrock** in production for enterprise compliance

## Troubleshooting

### Ollama Connection Issues
```bash
# Check if Ollama is running
curl http://localhost:11434/api/tags

# Start Ollama
ollama serve

# Verify model is installed
ollama list
```

### API Key Not Found
```bash
# Verify environment variable is set
echo $ANTHROPIC_API_KEY
echo $OPENAI_API_KEY
echo $OPENROUTER_API_KEY

# Set for current session
export ANTHROPIC_API_KEY=sk-ant-...
```

### AWS Credentials Issues
```bash
# Check AWS credentials
aws sts get-caller-identity

# Configure AWS CLI
aws configure

# Or use environment variables
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1
```

### Model Not Available
- Check the model name matches exactly (including version suffixes)
- For Bedrock: verify model is available in your AWS region
- For OpenRouter: check https://openrouter.ai/models for available models
- For Anthropic/OpenAI: check their documentation for current model names

## Advanced: Multiple Configurations

You can maintain different configurations for different scenarios:

```bash
# Fast local checks
gavel analyze --config .gavel/fast.yaml

# Production quality review
gavel analyze --config .gavel/production.yaml
```

Example `.gavel/fast.yaml`:
```yaml
provider:
  name: ollama
  ollama:
    model: qwen2.5-coder:7b
policies:
  # ... policies ...
```

Example `.gavel/production.yaml`:
```yaml
provider:
  name: anthropic
  anthropic:
    model: claude-sonnet-4-20250514
policies:
  # ... policies ...
```

## See Also

- `example-configs.yaml` - Complete configuration examples with all providers
- `CLAUDE.md` - Technical architecture and BAML details
- Anthropic API docs: https://docs.anthropic.com/
- OpenAI API docs: https://platform.openai.com/docs
- AWS Bedrock docs: https://docs.aws.amazon.com/bedrock/
- OpenRouter docs: https://openrouter.ai/docs
- Ollama docs: https://ollama.ai/
