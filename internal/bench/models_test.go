package bench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModels(t *testing.T) {
	resp := OpenRouterModelsResponse{
		Data: []OpenRouterModel{
			{
				ID:   "anthropic/claude-haiku-4.5",
				Name: "Claude Haiku 4.5",
				Pricing: OpenRouterPricing{
					Prompt:     "0.000001",
					Completion: "0.000005",
				},
				ContextLength: 200000,
			},
			{
				ID:   "google/gemini-3-flash-preview",
				Name: "Gemini 3 Flash",
				Pricing: OpenRouterPricing{
					Prompt:     "0.0000005",
					Completion: "0.000003",
				},
				ContextLength: 1048576,
			},
		},
	}
	body, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	models, err := FetchModels(context.Background(), "test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "anthropic/claude-haiku-4.5" {
		t.Errorf("expected claude-haiku-4.5, got %s", models[0].ID)
	}
	if models[0].InputPricePerM != 1.0 {
		t.Errorf("expected input price 1.0, got %f", models[0].InputPricePerM)
	}
	if models[0].OutputPricePerM != 5.0 {
		t.Errorf("expected output price 5.0, got %f", models[0].OutputPricePerM)
	}
}

func TestFetchModels_InvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	_, err := FetchModels(context.Background(), "bad-key", WithBaseURL(srv.URL))
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestValidateModelIDs(t *testing.T) {
	available := []ModelInfo{
		{ID: "anthropic/claude-haiku-4.5"},
		{ID: "google/gemini-3-flash-preview"},
		{ID: "deepseek/deepseek-v3.2"},
	}

	t.Run("all valid", func(t *testing.T) {
		valid, err := ValidateModelIDs(available, []string{"anthropic/claude-haiku-4.5", "deepseek/deepseek-v3.2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(valid) != 2 {
			t.Fatalf("expected 2 valid, got %d", len(valid))
		}
	})

	t.Run("some invalid", func(t *testing.T) {
		_, err := ValidateModelIDs(available, []string{"anthropic/claude-haiku-4.5", "openai/gpt-nonexistent"})
		if err == nil {
			t.Fatal("expected error for invalid model")
		}
	})
}
