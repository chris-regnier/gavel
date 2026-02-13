package rules

import (
	"os"
	"path/filepath"
	"testing"
)

const testRuleYAML = `rules:
  - id: "CUSTOM-001"
    name: "custom-rule"
    category: "security"
    pattern: 'custom_pattern'
    level: "error"
    confidence: 0.9
    message: "Custom rule triggered"
`

func writeRuleFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating dir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", filename, err)
	}
}

func TestLoadRules_DefaultsOnly(t *testing.T) {
	rules, err := LoadRules("", "")
	if err != nil {
		t.Fatalf("LoadRules() error: %v", err)
	}
	if len(rules) < 10 {
		t.Fatalf("expected at least 10 default rules, got %d", len(rules))
	}
}

func TestLoadRules_ProjectOverride(t *testing.T) {
	projectDir := t.TempDir()
	writeRuleFile(t, projectDir, "custom.yaml", testRuleYAML)

	rules, err := LoadRules("", projectDir)
	if err != nil {
		t.Fatalf("LoadRules() error: %v", err)
	}

	found := false
	for _, r := range rules {
		if r.ID == "CUSTOM-001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CUSTOM-001 from project dir, not found")
	}
	if len(rules) < 10 {
		t.Errorf("expected defaults plus custom rule, got %d rules", len(rules))
	}
}

func TestLoadRules_UserHomeRules(t *testing.T) {
	userDir := t.TempDir()
	writeRuleFile(t, userDir, "user-rules.yaml", testRuleYAML)

	rules, err := LoadRules(userDir, "")
	if err != nil {
		t.Fatalf("LoadRules() error: %v", err)
	}

	found := false
	for _, r := range rules {
		if r.ID == "CUSTOM-001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected CUSTOM-001 from user dir, not found")
	}
}

func TestLoadRules_ProjectOverridesHome(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	userYAML := `rules:
  - id: "OVERLAP-001"
    name: "user-version"
    category: "security"
    pattern: 'user_pattern'
    level: "warning"
    confidence: 0.5
    message: "User version"
`
	projectYAML := `rules:
  - id: "OVERLAP-001"
    name: "project-version"
    category: "security"
    pattern: 'project_pattern'
    level: "error"
    confidence: 0.9
    message: "Project version"
`
	writeRuleFile(t, userDir, "overlap.yaml", userYAML)
	writeRuleFile(t, projectDir, "overlap.yaml", projectYAML)

	rules, err := LoadRules(userDir, projectDir)
	if err != nil {
		t.Fatalf("LoadRules() error: %v", err)
	}

	for _, r := range rules {
		if r.ID == "OVERLAP-001" {
			if r.Name != "project-version" {
				t.Errorf("expected project-version to win, got %s", r.Name)
			}
			if r.Level != "error" {
				t.Errorf("expected level error, got %s", r.Level)
			}
			return
		}
	}
	t.Error("expected OVERLAP-001 rule, not found")
}

func TestLoadRules_MissingDirIsOK(t *testing.T) {
	rules, err := LoadRules("/nonexistent/user/path", "/nonexistent/project/path")
	if err != nil {
		t.Fatalf("expected no error for missing dirs, got: %v", err)
	}
	if len(rules) < 10 {
		t.Fatalf("expected at least 10 default rules, got %d", len(rules))
	}
}

func TestLoadRules_InvalidYAMLReturnsError(t *testing.T) {
	projectDir := t.TempDir()
	writeRuleFile(t, projectDir, "bad.yaml", "{{invalid yaml content")

	_, err := LoadRules("", projectDir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoadRules_MultipleFilesInDir(t *testing.T) {
	projectDir := t.TempDir()

	file1 := `rules:
  - id: "MULTI-001"
    name: "first-rule"
    category: "security"
    pattern: 'pattern_one'
    level: "error"
    confidence: 0.9
    message: "First rule"
`
	file2 := `rules:
  - id: "MULTI-002"
    name: "second-rule"
    category: "reliability"
    pattern: 'pattern_two'
    level: "warning"
    confidence: 0.8
    message: "Second rule"
`
	writeRuleFile(t, projectDir, "rules1.yaml", file1)
	writeRuleFile(t, projectDir, "rules2.yml", file2)

	rules, err := LoadRules("", projectDir)
	if err != nil {
		t.Fatalf("LoadRules() error: %v", err)
	}

	foundIDs := map[string]bool{}
	for _, r := range rules {
		foundIDs[r.ID] = true
	}

	if !foundIDs["MULTI-001"] {
		t.Error("expected MULTI-001 from rules1.yaml, not found")
	}
	if !foundIDs["MULTI-002"] {
		t.Error("expected MULTI-002 from rules2.yml, not found")
	}
}
