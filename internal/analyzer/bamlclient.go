package analyzer

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	baml_client "github.com/chris-regnier/gavel/baml_client"
	"github.com/chris-regnier/gavel/baml_client/types"
	"github.com/chris-regnier/gavel/internal/config"
)

// Ensure BAMLLiveClient satisfies the BAMLClient interface at compile time.
var _ BAMLClient = (*BAMLLiveClient)(nil)

// BAMLLiveClient wraps the generated BAML client to implement the BAMLClient interface.
type BAMLLiveClient struct {
	providerConfig config.ProviderConfig
}

// NewBAMLLiveClient creates a new live BAML client that calls the LLM via configured provider.
func NewBAMLLiveClient(cfg config.ProviderConfig) *BAMLLiveClient {
	return &BAMLLiveClient{
		providerConfig: cfg,
	}
}

// modelName returns the configured model name for the current provider.
func (c *BAMLLiveClient) modelName() string {
	switch c.providerConfig.Name {
	case "ollama":
		return c.providerConfig.Ollama.Model
	case "openrouter":
		return c.providerConfig.OpenRouter.Model
	case "anthropic":
		return c.providerConfig.Anthropic.Model
	case "bedrock":
		return c.providerConfig.Bedrock.Model
	case "openai":
		return c.providerConfig.OpenAI.Model
	default:
		return "unknown"
	}
}

// AnalyzeCode calls the appropriate BAML client based on provider config.
func (c *BAMLLiveClient) AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error) {
	model := c.modelName()
	ctx, span := analyzerTracer.Start(ctx, "chat "+model,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.request.model", model),
			attribute.String("gen_ai.provider.name", c.providerConfig.Name),
		),
	)
	defer span.End()

	var results []types.Finding
	var err error

	switch c.providerConfig.Name {
	case "ollama":
		results, err = c.analyzeWithOllama(ctx, code, policies, personaPrompt, additionalContext)
	case "openrouter":
		results, err = c.analyzeWithOpenRouter(ctx, code, policies, personaPrompt, additionalContext)
	case "anthropic":
		results, err = c.analyzeWithAnthropic(ctx, code, policies, personaPrompt, additionalContext)
	case "bedrock":
		results, err = c.analyzeWithBedrock(ctx, code, policies, personaPrompt, additionalContext)
	case "openai":
		results, err = c.analyzeWithOpenAI(ctx, code, policies, personaPrompt, additionalContext)
	default:
		return nil, fmt.Errorf("unknown provider: %s", c.providerConfig.Name)
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("analysis failed with %s: %w", c.providerConfig.Name, err)
	}

	return convertFindings(results), nil
}

func (c *BAMLLiveClient) analyzeWithOllama(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]types.Finding, error) {
	// Use WithClient to select the Ollama client and WithEnv to configure model/base_url
	env := map[string]string{
		"OLLAMA_MODEL": c.providerConfig.Ollama.Model,
	}
	// Only set base_url if non-empty, otherwise use system default
	if c.providerConfig.Ollama.BaseURL != "" {
		env["OLLAMA_BASE_URL"] = c.providerConfig.Ollama.BaseURL
	} else {
		env["OLLAMA_BASE_URL"] = "http://localhost:11434/v1"
	}
	return baml_client.AnalyzeCode(ctx, code, policies, personaPrompt, additionalContext,
		baml_client.WithClient("Ollama"),
		baml_client.WithEnv(env),
	)
}

func (c *BAMLLiveClient) analyzeWithOpenRouter(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]types.Finding, error) {
	// Use WithClient to select the OpenRouter client at runtime
	return baml_client.AnalyzeCode(ctx, code, policies, personaPrompt, additionalContext, baml_client.WithClient("OpenRouter"))
}

func (c *BAMLLiveClient) analyzeWithAnthropic(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]types.Finding, error) {
	// Use WithClient to select the Anthropic client and WithEnv to configure model
	env := map[string]string{
		"ANTHROPIC_MODEL": c.providerConfig.Anthropic.Model,
	}
	return baml_client.AnalyzeCode(ctx, code, policies, personaPrompt, additionalContext,
		baml_client.WithClient("Anthropic"),
		baml_client.WithEnv(env),
	)
}

func (c *BAMLLiveClient) analyzeWithBedrock(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]types.Finding, error) {
	// Use WithClient to select the Bedrock client and WithEnv to configure model and region
	env := map[string]string{
		"BEDROCK_MODEL":  c.providerConfig.Bedrock.Model,
		"BEDROCK_REGION": c.providerConfig.Bedrock.Region,
	}
	return baml_client.AnalyzeCode(ctx, code, policies, personaPrompt, additionalContext,
		baml_client.WithClient("Bedrock"),
		baml_client.WithEnv(env),
	)
}

func (c *BAMLLiveClient) analyzeWithOpenAI(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]types.Finding, error) {
	// Use WithClient to select the OpenAI client and WithEnv to configure model
	env := map[string]string{
		"OPENAI_MODEL": c.providerConfig.OpenAI.Model,
	}
	return baml_client.AnalyzeCode(ctx, code, policies, personaPrompt, additionalContext,
		baml_client.WithClient("OpenAI"),
		baml_client.WithEnv(env),
	)
}

func convertFindings(bamlFindings []types.Finding) []Finding {
	findings := make([]Finding, len(bamlFindings))
	for i, f := range bamlFindings {
		findings[i] = Finding{
			RuleID:         f.RuleId,
			Level:          f.Level,
			Message:        f.Message,
			FilePath:       f.FilePath,
			StartLine:      int(f.StartLine),
			EndLine:        int(f.EndLine),
			Recommendation: f.Recommendation,
			Explanation:    f.Explanation,
			Confidence:     f.Confidence,
		}
	}
	return findings
}
