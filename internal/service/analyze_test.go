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
	// readReturn, if non-nil, is returned from ReadSARIF instead of
	// echoing writtenSARIF — used to pre-seed a baseline log.
	readReturn *sarif.Log
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
	if m.readReturn != nil {
		return m.readReturn, nil
	}
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

// mockFindingClient returns a pre-configured finding list so we can
// exercise baseline comparison end-to-end through the service.
type mockFindingClient struct {
	findings []analyzer.Finding
}

func (m *mockFindingClient) AnalyzeCode(_ context.Context, _ string, _ string, _ string, _ string) ([]analyzer.Finding, error) {
	return m.findings, nil
}

func TestAnalyzeService_Analyze_WithBaseline(t *testing.T) {
	// Seed the "previous run" SARIF that will act as the baseline. The
	// baseline contains one error-level finding whose content matches
	// what the mock client will surface again.
	baselineLog := sarif.NewLog("gavel", "0.1.0")
	baselineLog.Runs[0].AutomationDetails = &sarif.RunAutomationDetails{Guid: "baseline-guid-xyz"}
	baselineLog.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "bug-detection",
			Level:   "error",
			Message: sarif.Message{Text: "seen"},
			Locations: []sarif.Location{{PhysicalLocation: sarif.PhysicalLocation{
				ArtifactLocation: sarif.ArtifactLocation{URI: "test.go"},
				Region: sarif.Region{
					StartLine: 5, EndLine: 5,
					Snippet: &sarif.ArtifactContent{Text: "password := \"hunter2\"\n"},
				},
			}}},
		},
	}
	sarif.SetContentFingerprint(&baselineLog.Runs[0].Results[0])

	ms := &mockStore{readReturn: baselineLog}

	// Current run: the mock returns one finding that matches the
	// baseline content (so it should come out "unchanged") and another
	// that does not (so it should come out "new"). The baseline's fixed
	// finding should be inherited as "absent" — but here baseline ==
	// current for the matched line, so we only exercise unchanged + new.
	svc := NewAnalyzeService(ms).WithClientFactory(func(_ config.ProviderConfig) analyzer.BAMLClient {
		return &mockFindingClient{
			findings: []analyzer.Finding{
				{RuleID: "bug-detection", Level: "error", Message: "seen", StartLine: 42, EndLine: 42},
				{RuleID: "bug-detection", Level: "warning", Message: "fresh", StartLine: 60, EndLine: 60},
			},
		}
	})

	req := AnalyzeRequest{
		Artifacts: []input.Artifact{{
			Path:    "test.go",
			Content: "line1\nline2\nline3\nline4\npassword := \"hunter2\"\nline6\nline7\nline8\nline9\nline10\nline11\nline12\nline13\nline14\nline15\nline16\nline17\nline18\nline19\nline20\nline21\nline22\nline23\nline24\nline25\nline26\nline27\nline28\nline29\nline30\nline31\nline32\nline33\nline34\nline35\nline36\nline37\nline38\nline39\nline40\nline41\npassword := \"hunter2\"\nline43\nline44\nline45\nline46\nline47\nline48\nline49\nline50\nline51\nline52\nline53\nline54\nline55\nline56\nline57\nline58\nline59\ndifferentFresh := 1\n",
			Kind:    input.KindFile,
		}},
		Config: config.Config{
			Provider: config.ProviderConfig{Name: "test"},
			Persona:  "code-reviewer",
			Policies: map[string]config.Policy{
				"bug-detection": {Enabled: true, Description: "Find bugs", Severity: "warning"},
			},
		},
		BaselineID: "any-id", // mockStore ignores the ID and returns readReturn
	}

	result, err := svc.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Baseline == nil {
		t.Fatal("expected Baseline summary to be populated")
	}
	if result.Baseline.New != 1 {
		t.Errorf("expected 1 new finding, got %d", result.Baseline.New)
	}
	if result.Baseline.Unchanged != 1 {
		t.Errorf("expected 1 unchanged finding, got %d", result.Baseline.Unchanged)
	}
	if result.Baseline.Absent != 0 {
		t.Errorf("expected 0 absent findings, got %d", result.Baseline.Absent)
	}

	// The stored SARIF should carry the baseline guid link back to the
	// previous run, and a fresh automation guid of its own.
	if ms.writtenSARIF == nil || len(ms.writtenSARIF.Runs) == 0 {
		t.Fatal("expected SARIF to be written to store")
	}
	run := ms.writtenSARIF.Runs[0]
	if run.BaselineGuid != "baseline-guid-xyz" {
		t.Errorf("BaselineGuid = %q, want %q", run.BaselineGuid, "baseline-guid-xyz")
	}
	if run.AutomationDetails == nil || run.AutomationDetails.Guid == "" {
		t.Error("expected AutomationDetails.Guid to be stamped on the new run")
	}
	if run.AutomationDetails != nil && run.AutomationDetails.Guid == "baseline-guid-xyz" {
		t.Error("new run should have its own guid, not share the baseline's")
	}
}

func TestAnalyzeService_Analyze_StampsAutomationDetailsWithoutBaseline(t *testing.T) {
	// Even without a baseline, every stored run should carry an
	// automation guid so the next run can reference it.
	ms := &mockStore{}
	svc := NewAnalyzeService(ms).WithClientFactory(func(_ config.ProviderConfig) analyzer.BAMLClient {
		return &mockBAMLClient{}
	})

	_, err := svc.Analyze(context.Background(), AnalyzeRequest{
		Artifacts: []input.Artifact{{Path: "test.go", Content: "package main\n", Kind: input.KindFile}},
		Config: config.Config{
			Provider: config.ProviderConfig{Name: "test"},
			Persona:  "code-reviewer",
			Policies: map[string]config.Policy{
				"bug-detection": {Enabled: true, Description: "Find bugs", Severity: "warning"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ms.writtenSARIF == nil || len(ms.writtenSARIF.Runs) == 0 {
		t.Fatal("expected SARIF to be written")
	}
	run := ms.writtenSARIF.Runs[0]
	if run.AutomationDetails == nil || run.AutomationDetails.Guid == "" {
		t.Error("expected AutomationDetails.Guid to be set even without --baseline")
	}
	if run.BaselineGuid != "" {
		t.Errorf("expected empty BaselineGuid without baseline, got %q", run.BaselineGuid)
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
	case err, ok := <-errCh:
		if ok {
			t.Fatalf("unexpected error: %v", err)
		}
	default:
	}
}
