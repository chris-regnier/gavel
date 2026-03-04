package bench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCase(t *testing.T) {
	// Create a temp corpus case directory
	dir := t.TempDir()
	caseDir := filepath.Join(dir, "sql-injection")
	os.MkdirAll(caseDir, 0o755)

	os.WriteFile(filepath.Join(caseDir, "source.go"), []byte(`package main
import "database/sql"
func query(db *sql.DB, input string) {
    db.Query("SELECT * FROM users WHERE name = '" + input + "'")
}`), 0o644)

	os.WriteFile(filepath.Join(caseDir, "expected.yaml"), []byte(`findings:
  - rule_id: "any"
    severity: error
    line_range: [4, 4]
    category: sql-injection
    must_find: true
false_positives: 0
`), 0o644)

	os.WriteFile(filepath.Join(caseDir, "metadata.yaml"), []byte(`name: SQL Injection via String Concatenation
language: go
category: security
difficulty: easy
description: Direct string concatenation in SQL query
`), 0o644)

	c, err := LoadCase(caseDir)
	if err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if c.Name != "sql-injection" {
		t.Errorf("Name = %q, want sql-injection", c.Name)
	}
	if len(c.ExpectedFindings) != 1 {
		t.Fatalf("ExpectedFindings = %d, want 1", len(c.ExpectedFindings))
	}
	if c.ExpectedFindings[0].MustFind != true {
		t.Error("MustFind should be true")
	}
	if c.SourcePath == "" {
		t.Error("SourcePath should not be empty")
	}
	if c.Metadata.Language != "go" {
		t.Errorf("Language = %q, want go", c.Metadata.Language)
	}
}

func TestLoadCorpus(t *testing.T) {
	dir := t.TempDir()
	// Create two language dirs with one case each
	for _, lang := range []string{"go", "python"} {
		caseDir := filepath.Join(dir, lang, "test-case")
		os.MkdirAll(caseDir, 0o755)
		os.WriteFile(filepath.Join(caseDir, "source.go"), []byte("package main"), 0o644)
		os.WriteFile(filepath.Join(caseDir, "expected.yaml"), []byte("findings: []\nfalse_positives: 0\n"), 0o644)
		os.WriteFile(filepath.Join(caseDir, "metadata.yaml"), []byte("name: test\nlanguage: "+lang+"\ncategory: test\ndifficulty: easy\n"), 0o644)
	}

	corpus, err := LoadCorpus(dir)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(corpus.Cases) != 2 {
		t.Errorf("Cases = %d, want 2", len(corpus.Cases))
	}
}
