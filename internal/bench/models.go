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

type OpenRouterModelsResponse struct {
	Data []OpenRouterModel `json:"data"`
}

type OpenRouterModel struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Pricing       OpenRouterPricing `json:"pricing"`
	ContextLength int               `json:"context_length"`
}

type OpenRouterPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

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

type FetchOption func(*fetchOptions)

func WithBaseURL(url string) FetchOption {
	return func(o *fetchOptions) { o.baseURL = url }
}

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
		inputPrice, _ := strconv.ParseFloat(m.Pricing.Prompt, 64)
		outputPrice, _ := strconv.ParseFloat(m.Pricing.Completion, 64)
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

func SortByPrice(models []ModelInfo) {
	sort.Slice(models, func(i, j int) bool {
		return models[i].InputPricePerM < models[j].InputPricePerM
	})
}

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
