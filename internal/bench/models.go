package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

const defaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// OpenRouterModelsResponse is the top-level response from the OpenRouter /models endpoint.
type OpenRouterModelsResponse struct {
	Data []OpenRouterModel `json:"data"`
}

// OpenRouterModel is a single model entry returned by the OpenRouter API.
type OpenRouterModel struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Pricing       OpenRouterPricing `json:"pricing"`
	ContextLength int               `json:"context_length"`
}

// OpenRouterPricing holds per-token pricing strings as returned by the OpenRouter API.
// Values are expressed as cost per token (not per million tokens).
type OpenRouterPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// ModelInfo is the normalised representation of a model used throughout the bench package.
// Prices are stored as cost per million tokens.
type ModelInfo struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	InputPricePerM  float64 `json:"input_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`
	ContextLength   int     `json:"context_length"`
}

type fetchOptions struct {
	baseURL string
}

// FetchOption is a functional option for FetchModels.
type FetchOption func(*fetchOptions)

// WithBaseURL overrides the OpenRouter base URL used by FetchModels.
// Primarily useful in tests.
func WithBaseURL(url string) FetchOption {
	return func(o *fetchOptions) { o.baseURL = url }
}

// FetchModels retrieves the full model catalogue from the OpenRouter API and
// returns a slice of ModelInfo with prices normalised to cost per million tokens.
func FetchModels(ctx context.Context, apiKey string, opts ...FetchOption) ([]ModelInfo, error) {
	o := &fetchOptions{baseURL: defaultOpenRouterBaseURL}
	for _, opt := range opts {
		opt(o)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter API returned %d", resp.StatusCode)
	}

	var result OpenRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		inputPrice, err := strconv.ParseFloat(m.Pricing.Prompt, 64)
		if err != nil && m.Pricing.Prompt != "" {
			return nil, fmt.Errorf("parsing input price for model %s: %w", m.ID, err)
		}
		outputPrice, err := strconv.ParseFloat(m.Pricing.Completion, 64)
		if err != nil && m.Pricing.Completion != "" {
			return nil, fmt.Errorf("parsing output price for model %s: %w", m.ID, err)
		}
		models = append(models, ModelInfo{
			ID:              m.ID,
			Name:            m.Name,
			InputPricePerM:  inputPrice * 1_000_000,
			OutputPricePerM: outputPrice * 1_000_000,
			ContextLength:   m.ContextLength,
		})
	}
	return models, nil
}

// SortByPrice sorts models in-place by ascending input price per million tokens.
func SortByPrice(models []ModelInfo) {
	sort.Slice(models, func(i, j int) bool {
		return models[i].InputPricePerM < models[j].InputPricePerM
	})
}

// ValidateModelIDs checks that every requested model ID exists in the available
// catalogue. It returns a map of valid models and a non-nil error listing any
// IDs that were not found. The valid map is always populated for the IDs that
// did match, allowing callers to proceed with partial results if desired.
func ValidateModelIDs(available []ModelInfo, requested []string) (map[string]ModelInfo, error) {
	lookup := make(map[string]ModelInfo, len(available))
	for _, m := range available {
		lookup[m.ID] = m
	}

	valid := make(map[string]ModelInfo, len(requested))
	var invalid []string
	for _, id := range requested {
		if m, ok := lookup[id]; ok {
			valid[id] = m
		} else {
			invalid = append(invalid, id)
		}
	}

	if len(invalid) > 0 {
		return valid, fmt.Errorf("models not found on OpenRouter: %v", invalid)
	}
	return valid, nil
}
