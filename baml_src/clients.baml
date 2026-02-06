// OpenRouter client for gavel analysis
client<llm> OpenRouter {
  provider "openai-generic"
  retry_policy Exponential
  options {
    base_url "https://openrouter.ai/api/v1"
    api_key env.OPENROUTER_API_KEY
    model "anthropic/claude-sonnet-4"
  }
}

// Ollama client for local LLM analysis
client<llm> Ollama {
  provider "openai-generic"
  retry_policy Exponential
  options {
    base_url env.OLLAMA_BASE_URL
    model env.OLLAMA_MODEL
  }
}

// Anthropic client for Claude API
client<llm> Anthropic {
  provider "anthropic"
  retry_policy Exponential
  options {
    api_key env.ANTHROPIC_API_KEY
    model env.ANTHROPIC_MODEL
  }
}

// AWS Bedrock client for Claude models on Bedrock
client<llm> Bedrock {
  provider "aws-bedrock"
  retry_policy Exponential
  options {
    region env.BEDROCK_REGION
    model env.BEDROCK_MODEL
  }
}

// OpenAI client for GPT models
client<llm> OpenAI {
  provider "openai"
  retry_policy Exponential
  options {
    api_key env.OPENAI_API_KEY
    model env.OPENAI_MODEL
  }
}

retry_policy Exponential {
  max_retries 2
  strategy {
    type exponential_backoff
    delay_ms 300
    multiplier 1.5
    max_delay_ms 10000
  }
}
