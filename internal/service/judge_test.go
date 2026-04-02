// internal/service/judge_test.go
package service

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestJudgeService_Judge(t *testing.T) {
	ms := &mockStore{}
	ctx := context.Background()

	// Write a SARIF log so there's something to judge
	sarifLog := sarif.Assemble(nil, nil, "directory", "code-reviewer")
	id, err := ms.WriteSARIF(ctx, sarifLog)
	if err != nil {
		t.Fatalf("writing SARIF: %v", err)
	}

	svc := NewJudgeService(ms, "")
	verdict, err := svc.Judge(ctx, JudgeRequest{ResultID: id})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.Decision == "" {
		t.Fatal("expected non-empty decision")
	}
}
