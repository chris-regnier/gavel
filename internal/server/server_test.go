// internal/server/server_test.go
package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/server"
	"github.com/chris-regnier/gavel/internal/service"
	"github.com/chris-regnier/gavel/internal/store"
)

// mockBAMLClient returns no findings (fast, deterministic).
type mockBAMLClient struct{}

func (m *mockBAMLClient) AnalyzeCode(_ context.Context, _ string, _ string, _ string, _ string) ([]analyzer.Finding, error) {
	return []analyzer.Finding{
		{RuleID: "TEST001", Level: "warning", Message: "test finding", Confidence: 0.9},
	}, nil
}

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	fs := store.NewFileStore(t.TempDir())
	analyzeSvc := service.NewAnalyzeService(fs).WithClientFactory(
		func(_ config.ProviderConfig) analyzer.BAMLClient {
			return &mockBAMLClient{}
		},
	)
	judgeSvc := service.NewJudgeService(fs)

	router := server.NewRouter(server.RouterConfig{
		AnalyzeService: analyzeSvc,
		JudgeService:   judgeSvc,
		Store:          fs,
		AuthKeys:       map[string]string{"test-key": "test-tenant"},
		MaxConcurrent:  5,
	})

	return httptest.NewServer(router)
}

func TestIntegration_AnalyzeSync(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
		"artifacts": [{"path": "test.go", "content": "package main\n", "kind": "file"}],
		"config": {
			"provider": {"name": "test"},
			"persona": "code-reviewer",
			"policies": {"test": {"enabled": true, "description": "Test", "severity": "warning"}}
		}
	}`

	req, _ := http.NewRequest("POST", ts.URL+"/v1/analyze", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result service.AnalyzeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if result.ResultID == "" {
		t.Error("expected non-empty result ID")
	}
}

func TestIntegration_Health(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_Unauthorized(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{"artifacts": [], "config": {"provider": {"name": "test"}, "persona": "code-reviewer"}}`
	req, _ := http.NewRequest("POST", ts.URL+"/v1/analyze", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer wrong-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestIntegration_AnalyzeStream(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	body := `{
		"artifacts": [{"path": "test.go", "content": "package main\n", "kind": "file"}],
		"config": {
			"provider": {"name": "test"},
			"persona": "code-reviewer",
			"policies": {"test": {"enabled": true, "description": "Test", "severity": "warning"}}
		}
	}`

	req, _ := http.NewRequest("POST", ts.URL+"/v1/analyze/stream", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	// Read SSE events
	var events []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			events = append(events, strings.TrimPrefix(line, "event: "))
		}
	}

	hasTier := false
	hasComplete := false
	for _, e := range events {
		if e == "tier" {
			hasTier = true
		}
		if e == "complete" {
			hasComplete = true
		}
	}

	if !hasTier {
		t.Error("expected at least one 'tier' SSE event")
	}
	if !hasComplete {
		t.Error("expected a 'complete' SSE event")
	}
}
