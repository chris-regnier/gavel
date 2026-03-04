package bench

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/sarif"
)

type mockJudgeClient struct{}

func (m *mockJudgeClient) AnalyzeCode(ctx context.Context, code, policies, persona, additional string) ([]analyzer.Finding, error) {
	return []analyzer.Finding{
		{Message: `{"score": 4, "label": "valid", "reasoning": "Real SQL injection vulnerability"}`, Confidence: 0.95},
	}, nil
}

func TestJudgeFinding(t *testing.T) {
	finding := sarif.Result{
		RuleID:  "SEC001",
		Level:   "error",
		Message: sarif.Message{Text: "SQL injection via string concatenation"},
		Properties: map[string]interface{}{
			"gavel/confidence":     0.9,
			"gavel/recommendation": "Use parameterized queries",
			"gavel/explanation":    "User input concatenated into SQL",
		},
	}
	sourceCode := `package main
import "database/sql"
func query(db *sql.DB, input string) {
    db.Query("SELECT * FROM users WHERE name = '" + input + "'")
}`

	verdict, err := JudgeFinding(context.Background(), &mockJudgeClient{}, finding, sourceCode)
	if err != nil {
		t.Fatalf("JudgeFinding: %v", err)
	}
	if verdict.Score < 1 || verdict.Score > 5 {
		t.Errorf("Score = %d, want 1-5", verdict.Score)
	}
	if verdict.Label == "" {
		t.Error("Label should not be empty")
	}
}
