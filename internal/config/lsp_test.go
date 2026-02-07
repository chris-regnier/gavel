package config

import (
	"testing"
)

func TestLSPConfigDefaults(t *testing.T) {
	cfg := SystemDefaults()

	// Test Watcher defaults
	if cfg.LSP.Watcher.DebounceDuration != "5m" {
		t.Errorf("expected debounce duration 5m, got %s", cfg.LSP.Watcher.DebounceDuration)
	}

	expectedWatchPatterns := []string{
		"**/*.go",
		"**/*.py",
		"**/*.ts",
		"**/*.tsx",
		"**/*.js",
		"**/*.jsx",
	}
	if len(cfg.LSP.Watcher.WatchPatterns) != len(expectedWatchPatterns) {
		t.Errorf("expected %d watch patterns, got %d", len(expectedWatchPatterns), len(cfg.LSP.Watcher.WatchPatterns))
	}
	for i, pattern := range expectedWatchPatterns {
		if i >= len(cfg.LSP.Watcher.WatchPatterns) || cfg.LSP.Watcher.WatchPatterns[i] != pattern {
			t.Errorf("watch pattern %d: expected %s, got %s", i, pattern, cfg.LSP.Watcher.WatchPatterns[i])
		}
	}

	expectedIgnorePatterns := []string{
		"**/node_modules/**",
		"**/.git/**",
		"**/vendor/**",
		"**/.gavel/**",
	}
	if len(cfg.LSP.Watcher.IgnorePatterns) != len(expectedIgnorePatterns) {
		t.Errorf("expected %d ignore patterns, got %d", len(expectedIgnorePatterns), len(cfg.LSP.Watcher.IgnorePatterns))
	}
	for i, pattern := range expectedIgnorePatterns {
		if i >= len(cfg.LSP.Watcher.IgnorePatterns) || cfg.LSP.Watcher.IgnorePatterns[i] != pattern {
			t.Errorf("ignore pattern %d: expected %s, got %s", i, pattern, cfg.LSP.Watcher.IgnorePatterns[i])
		}
	}

	// Test Analysis defaults
	if cfg.LSP.Analysis.ParallelFiles != 3 {
		t.Errorf("expected parallel_files 3, got %d", cfg.LSP.Analysis.ParallelFiles)
	}
	if cfg.LSP.Analysis.Priority != "recent" {
		t.Errorf("expected priority 'recent', got %s", cfg.LSP.Analysis.Priority)
	}

	// Test Cache defaults
	if cfg.LSP.Cache.TTL != "168h" {
		t.Errorf("expected TTL 168h, got %s", cfg.LSP.Cache.TTL)
	}
	if cfg.LSP.Cache.MaxSizeMB != 500 {
		t.Errorf("expected max_size_mb 500, got %d", cfg.LSP.Cache.MaxSizeMB)
	}
}
