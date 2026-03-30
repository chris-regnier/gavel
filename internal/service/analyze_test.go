package service

import (
	"context"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	writtenSARIF *sarif.Log
	writtenID    string
}

func (m *mockStore) WriteSARIF(_ context.Context, doc *sarif.Log) (string, error) {
	m.writtenSARIF = doc
	m.writtenID = "test-result-id"
	return m.writtenID, nil
}

func (m *mockStore) WriteVerdict(_ context.Context, _ string, _ *store.Verdict) error {
	return nil
}

func (m *mockStore) ReadSARIF(_ context.Context, _ string) (*sarif.Log, error) {
	return m.writtenSARIF, nil
}

func (m *mockStore) ReadVerdict(_ context.Context, _ string) (*store.Verdict, error) {
	return nil, nil
}

func (m *mockStore) List(_ context.Context) ([]string, error) {
	return []string{m.writtenID}, nil
}

// mockBAMLClient implements analyzer.BAMLClient for testing.
type mockBAMLClient struct{}

func (m *mockBAMLClient) AnalyzeCode(_ context.Context, _ string, _ string, _ string, _ string) ([]analyzer.Finding, error) {
	return nil, nil
}

func TestAnalyzeService_Analyze(t *testing.T) {
	ms := &mockStore{}
	svc := NewAnalyzeService(ms).WithClientFactory(func(_ config.ProviderConfig) analyzer.BAMLClient {
		return &mockBAMLClient{}
	})

	req := AnalyzeRequest{
		Artifacts: []input.Artifact{
			{Path: "test.go", Content: "package main\n", Kind: input.KindFile},
		},
		Config: config.Config{
			Provider: config.ProviderConfig{Name: "test"},
			Persona:  "code-reviewer",
			Policies: map[string]config.Policy{
				"bug-detection": {Enabled: true, Description: "Find bugs", Severity: "warning"},
			},
		},
	}

	result, err := svc.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ResultID == "" {
		t.Fatal("expected non-empty result ID")
	}
	if ms.writtenSARIF == nil {
		t.Fatal("expected SARIF to be written to store")
	}
}

func TestAnalyzeService_AnalyzeStream(t *testing.T) {
	ms := &mockStore{}
	svc := NewAnalyzeService(ms).WithClientFactory(func(_ config.ProviderConfig) analyzer.BAMLClient {
		return &mockBAMLClient{}
	})

	req := AnalyzeRequest{
		Artifacts: []input.Artifact{
			{Path: "test.go", Content: "package main\n", Kind: input.KindFile},
		},
		Config: config.Config{
			Provider: config.ProviderConfig{Name: "test"},
			Persona:  "code-reviewer",
			Policies: map[string]config.Policy{
				"bug-detection": {Enabled: true, Description: "Find bugs", Severity: "warning"},
			},
		},
	}

	tierCh, resultCh, errCh := svc.AnalyzeStream(context.Background(), req)

	var tiers []TierResult
	for tr := range tierCh {
		tiers = append(tiers, tr)
	}

	if len(tiers) == 0 {
		t.Fatal("expected at least one tier result")
	}

	for _, tr := range tiers {
		if tr.Tier == "" {
			t.Error("tier result missing tier name")
		}
	}

	// Result should arrive
	select {
	case result := <-resultCh:
		if result.ResultID == "" {
			t.Error("expected non-empty result ID")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for result")
	}

	// No fatal errors
	select {
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	default:
	}
}
