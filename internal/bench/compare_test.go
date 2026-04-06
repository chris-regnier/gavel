package bench

import (
	"context"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
)

// mockDelayClient returns findings after a fixed delay.
type mockDelayClient struct {
	delay    time.Duration
	findings []analyzer.Finding
}

func (m *mockDelayClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
	time.Sleep(m.delay)
	return m.findings, nil
}

func TestBenchClient_RecordsTiming(t *testing.T) {
	inner := &mockDelayClient{
		delay: 50 * time.Millisecond,
		findings: []analyzer.Finding{
			{RuleID: "test-rule", Message: "test finding", Level: "warning", Confidence: 0.8},
		},
	}
	bc := NewBenchClient(inner, ModelInfo{
		ID:              "test/model",
		InputPricePerM:  1.0,
		OutputPricePerM: 5.0,
	})

	code := "func main() { fmt.Println(password) }"
	policies := "check for hardcoded secrets"

	findings, err := bc.AnalyzeCode(context.Background(), code, policies, "persona", "")
	if err != nil {
		t.Fatalf("AnalyzeCode: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	calls := bc.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call record, got %d", len(calls))
	}
	call := calls[0]
	if call.LatencyMs < 50 {
		t.Errorf("latency too low: %d ms", call.LatencyMs)
	}
	if call.InputTokensEst <= 0 {
		t.Errorf("expected positive input token estimate, got %d", call.InputTokensEst)
	}
	if call.OutputTokensEst <= 0 {
		t.Errorf("expected positive output token estimate, got %d", call.OutputTokensEst)
	}
}
