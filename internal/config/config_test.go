package config

import (
	"os"
	"testing"
)

func TestMergePolicies_HigherTierOverrides(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Description: "Handle errors",
				Severity:    "warning",
				Instruction: "Check error handling",
				Enabled:     true,
			},
		},
	}
	project := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Severity: "error",
			},
		},
	}
	merged := MergeConfigs(system, project)
	pol := merged.Policies["error-handling"]
	if pol.Severity != "error" {
		t.Errorf("expected severity 'error', got %q", pol.Severity)
	}
	if pol.Description != "Handle errors" {
		t.Errorf("expected description preserved, got %q", pol.Description)
	}
	if !pol.Enabled {
		t.Error("expected enabled to remain true")
	}
}

func TestMergePolicies_HigherTierAddsNew(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {Description: "Handle errors", Severity: "warning", Instruction: "Check error handling", Enabled: true},
		},
	}
	project := &Config{
		Policies: map[string]Policy{
			"function-length": {Description: "Keep functions short", Severity: "note", Instruction: "Flag long functions", Enabled: true},
		},
	}
	merged := MergeConfigs(system, project)
	if len(merged.Policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(merged.Policies))
	}
}

func TestMergePolicies_DisablePolicy(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {Description: "Handle errors", Severity: "warning", Instruction: "Check error handling", Enabled: true},
		},
	}
	project := &Config{
		Policies: map[string]Policy{
			"error-handling": {Enabled: false},
		},
	}
	merged := MergeConfigs(system, project)
	if merged.Policies["error-handling"].Enabled {
		t.Error("expected policy to be disabled")
	}
}

func TestLoadFromFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/policies.yaml"
	os.WriteFile(path, []byte("policies:\n  test-policy:\n    description: \"Test\"\n    severity: \"warning\"\n    instruction: \"Do the thing\"\n    enabled: true\n"), 0644)
	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(cfg.Policies))
	}
}

func TestLoadFromFile_Missing(t *testing.T) {
	cfg, err := LoadFromFile("/nonexistent/path.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Error("expected nil config for missing file")
	}
}

func TestLoadTiered(t *testing.T) {
	dir := t.TempDir()
	machineConf := dir + "/machine.yaml"
	os.WriteFile(machineConf, []byte("policies:\n  error-handling:\n    severity: \"error\"\n"), 0644)
	projectConf := dir + "/project.yaml"
	os.WriteFile(projectConf, []byte("policies:\n  custom-rule:\n    description: \"Custom\"\n    severity: \"warning\"\n    instruction: \"Check custom thing\"\n    enabled: true\n"), 0644)

	cfg, err := LoadTiered(machineConf, projectConf)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Policies["error-handling"].Severity != "error" {
		t.Errorf("expected machine override severity 'error', got %q", cfg.Policies["error-handling"].Severity)
	}
	if _, ok := cfg.Policies["custom-rule"]; !ok {
		t.Error("expected project policy 'custom-rule'")
	}
	if _, ok := cfg.Policies["function-length"]; !ok {
		t.Error("expected system default 'function-length'")
	}
}
