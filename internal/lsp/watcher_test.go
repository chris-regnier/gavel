// internal/lsp/watcher_test.go
package lsp

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncedWatcher(t *testing.T) {
	var triggerCount atomic.Int32
	onTrigger := func(files []string) {
		triggerCount.Add(1)
	}

	w := NewDebouncedWatcher(50*time.Millisecond, onTrigger)

	// Simulate rapid file changes
	w.FileChanged("a.go")
	w.FileChanged("b.go")
	w.FileChanged("c.go")

	// Wait less than debounce period - should not trigger
	time.Sleep(30 * time.Millisecond)
	if triggerCount.Load() != 0 {
		t.Fatal("triggered too early")
	}

	// Wait for debounce to complete
	time.Sleep(50 * time.Millisecond)
	if triggerCount.Load() != 1 {
		t.Fatalf("expected 1 trigger, got %d", triggerCount.Load())
	}

	w.Stop()
}

func TestDebouncedWatcherMultipleBatches(t *testing.T) {
	var batches [][]string
	var mu sync.Mutex
	onTrigger := func(files []string) {
		mu.Lock()
		batches = append(batches, files)
		mu.Unlock()
	}

	w := NewDebouncedWatcher(30*time.Millisecond, onTrigger)

	// First batch
	w.FileChanged("a.go")
	time.Sleep(50 * time.Millisecond)

	// Second batch
	w.FileChanged("b.go")
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	mu.Unlock()

	w.Stop()
}

func TestDebouncedWatcherWithConfig(t *testing.T) {
	var files []string
	var mu sync.Mutex
	onTrigger := func(f []string) {
		mu.Lock()
		files = append(files, f...)
		mu.Unlock()
	}

	config := WatcherConfig{
		DebounceDuration: 30 * time.Millisecond,
		ParallelFiles:    2,
		WatchPatterns:    []string{"**/*.go"},
		IgnorePatterns:   []string{"**/vendor/**"},
	}

	w := NewDebouncedWatcherWithConfig(config, onTrigger)

	w.FileChanged("main.go")
	w.FileChanged("util.go")
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
	mu.Unlock()

	w.Stop()
}

func TestDebouncedWatcherUpdateConfig(t *testing.T) {
	onTrigger := func(files []string) {}
	w := NewDebouncedWatcher(50*time.Millisecond, onTrigger)

	newConfig := WatcherConfig{
		DebounceDuration: 100 * time.Millisecond,
		ParallelFiles:    5,
	}

	w.UpdateConfig(newConfig)

	cfg := w.Config()
	if cfg.DebounceDuration != 100*time.Millisecond {
		t.Errorf("DebounceDuration = %v, want 100ms", cfg.DebounceDuration)
	}
	if cfg.ParallelFiles != 5 {
		t.Errorf("ParallelFiles = %d, want 5", cfg.ParallelFiles)
	}

	w.Stop()
}

func TestShouldWatchPath(t *testing.T) {
	watchPatterns := []string{"**/*.go", "**/*.py", "**/*.ts"}
	ignorePatterns := []string{"**/node_modules/**", "**/.git/**", "**/vendor/**"}

	tests := []struct {
		path string
		want bool
	}{
		{"file:///project/main.go", true},
		{"file:///project/src/handler.go", true},
		{"file:///project/script.py", true},
		{"file:///project/index.ts", true},
		{"file:///project/README.md", false},
		{"file:///project/node_modules/pkg/index.js", false},
		{"file:///project/.git/config", false},
		{"file:///project/vendor/pkg/main.go", false},
		{"/workspace/main.go", true},
		{"/workspace/node_modules/main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := ShouldWatchPath(tt.path, watchPatterns, ignorePatterns)
			if got != tt.want {
				t.Errorf("ShouldWatchPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		// Extension patterns
		{"/project/main.go", "**/*.go", true},
		{"/project/src/handler.go", "**/*.go", true},
		{"/project/main.py", "**/*.go", false},

		// Directory patterns
		{"/project/node_modules/pkg/index.js", "**/node_modules/**", true},
		{"/project/.git/config", "**/.git/**", true},
		{"/project/vendor/pkg.go", "**/vendor/**", true},
		{"/project/src/main.go", "**/vendor/**", false},

		// Simple extension patterns
		{"main.go", "*.go", true},
		{"main.py", "*.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.pattern, func(t *testing.T) {
			got := matchGlobPattern(tt.path, tt.pattern)
			if got != tt.want {
				t.Errorf("matchGlobPattern(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"5m", 5 * time.Minute, false},
		{"300ms", 300 * time.Millisecond, false},
		{"1h", time.Hour, false},
		{"168h", 168 * time.Hour, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
