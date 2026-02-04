// internal/store/filestore_test.go
package store

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestFileStore_WriteAndReadSARIF(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	ctx := context.Background()

	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = append(log.Runs[0].Results, sarif.Result{
		RuleID:  "test-rule",
		Level:   "warning",
		Message: sarif.Message{Text: "test finding"},
	})

	id, err := fs.WriteSARIF(ctx, log)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	loaded, err := fs.ReadSARIF(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(loaded.Runs[0].Results))
	}
	if loaded.Runs[0].Results[0].RuleID != "test-rule" {
		t.Errorf("expected ruleId 'test-rule', got %q", loaded.Runs[0].Results[0].RuleID)
	}
}

func TestFileStore_WriteAndReadVerdict(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	ctx := context.Background()

	log := sarif.NewLog("gavel", "0.1.0")
	id, err := fs.WriteSARIF(ctx, log)
	if err != nil {
		t.Fatal(err)
	}

	verdict := &Verdict{
		Decision: "merge",
		Reason:   "No issues found",
	}

	if err := fs.WriteVerdict(ctx, id, verdict); err != nil {
		t.Fatal(err)
	}

	loaded, err := fs.ReadVerdict(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Decision != "merge" {
		t.Errorf("expected decision 'merge', got %q", loaded.Decision)
	}
}

func TestFileStore_List(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	ctx := context.Background()

	log := sarif.NewLog("gavel", "0.1.0")
	fs.WriteSARIF(ctx, log)
	fs.WriteSARIF(ctx, log)

	ids, err := fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 results, got %d", len(ids))
	}
}
