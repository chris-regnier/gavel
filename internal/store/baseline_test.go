package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestLoadBaseline_FromStoreID(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	ctx := context.Background()

	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{{RuleID: "r1"}}
	id, err := fs.WriteSARIF(ctx, log)
	if err != nil {
		t.Fatalf("WriteSARIF: %v", err)
	}

	got, err := LoadBaseline(ctx, fs, id)
	if err != nil {
		t.Fatalf("LoadBaseline by id: %v", err)
	}
	if len(got.Runs) == 0 || len(got.Runs[0].Results) != 1 || got.Runs[0].Results[0].RuleID != "r1" {
		t.Errorf("unexpected log round-trip: %+v", got)
	}
}

func TestLoadBaseline_FromFilePath(t *testing.T) {
	dir := t.TempDir()
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{{RuleID: "file-rule"}}
	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(dir, "prev.sarif.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Use a filestore that points at a DIFFERENT directory to prove
	// the file path takes precedence over the store ID lookup.
	fs := NewFileStore(t.TempDir())

	got, err := LoadBaseline(context.Background(), fs, path)
	if err != nil {
		t.Fatalf("LoadBaseline by path: %v", err)
	}
	if len(got.Runs) == 0 || got.Runs[0].Results[0].RuleID != "file-rule" {
		t.Errorf("unexpected log: %+v", got)
	}
}

func TestLoadBaseline_MissingID(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	if _, err := LoadBaseline(context.Background(), fs, "does-not-exist"); err == nil {
		t.Error("expected error for missing id")
	}
}

func TestLoadBaseline_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bogus.json")
	if err := os.WriteFile(path, []byte("not-json"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	fs := NewFileStore(t.TempDir())
	if _, err := LoadBaseline(context.Background(), fs, path); err == nil {
		t.Error("expected error for invalid JSON in baseline file")
	}
}
