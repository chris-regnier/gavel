// internal/lsp/watcher.go
package lsp

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WatcherConfig holds configuration for the debounced watcher
type WatcherConfig struct {
	DebounceDuration time.Duration
	ParallelFiles    int
	WatchPatterns    []string
	IgnorePatterns   []string
}

// DefaultWatcherConfig returns sensible defaults for the watcher
func DefaultWatcherConfig() WatcherConfig {
	return WatcherConfig{
		DebounceDuration: 300 * time.Millisecond,
		ParallelFiles:    3,
		WatchPatterns: []string{
			"**/*.go",
			"**/*.py",
			"**/*.ts",
			"**/*.tsx",
			"**/*.js",
			"**/*.jsx",
		},
		IgnorePatterns: []string{
			"**/node_modules/**",
			"**/.git/**",
			"**/vendor/**",
			"**/.gavel/**",
		},
	}
}

// DebouncedWatcher batches file changes and triggers analysis after quiet period
type DebouncedWatcher struct {
	config    WatcherConfig
	onTrigger func(files []string)

	mu      sync.Mutex
	pending map[string]struct{}
	timer   *time.Timer
	stopCh  chan struct{}
	stopped bool
}

// NewDebouncedWatcher creates a watcher with default configuration
func NewDebouncedWatcher(debounce time.Duration, onTrigger func(files []string)) *DebouncedWatcher {
	if onTrigger == nil {
		panic("onTrigger callback cannot be nil")
	}
	config := DefaultWatcherConfig()
	config.DebounceDuration = debounce
	return &DebouncedWatcher{
		config:    config,
		onTrigger: onTrigger,
		pending:   make(map[string]struct{}),
		stopCh:    make(chan struct{}),
	}
}

// NewDebouncedWatcherWithConfig creates a watcher with custom configuration
func NewDebouncedWatcherWithConfig(config WatcherConfig, onTrigger func(files []string)) *DebouncedWatcher {
	if onTrigger == nil {
		panic("onTrigger callback cannot be nil")
	}
	if config.DebounceDuration == 0 {
		config.DebounceDuration = DefaultWatcherConfig().DebounceDuration
	}
	if config.ParallelFiles == 0 {
		config.ParallelFiles = DefaultWatcherConfig().ParallelFiles
	}
	return &DebouncedWatcher{
		config:    config,
		onTrigger: onTrigger,
		pending:   make(map[string]struct{}),
		stopCh:    make(chan struct{}),
	}
}

// UpdateConfig updates the watcher configuration
func (w *DebouncedWatcher) UpdateConfig(config WatcherConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if config.DebounceDuration > 0 {
		w.config.DebounceDuration = config.DebounceDuration
	}
	if config.ParallelFiles > 0 {
		w.config.ParallelFiles = config.ParallelFiles
	}
	if len(config.WatchPatterns) > 0 {
		w.config.WatchPatterns = config.WatchPatterns
	}
	if len(config.IgnorePatterns) > 0 {
		w.config.IgnorePatterns = config.IgnorePatterns
	}
}

// Config returns the current configuration
func (w *DebouncedWatcher) Config() WatcherConfig {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.config
}

// FileChanged queues a file for analysis
func (w *DebouncedWatcher) FileChanged(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stopped {
		return
	}

	w.pending[path] = struct{}{}

	// Reset timer on each change
	if w.timer != nil {
		w.timer.Stop()
	}

	w.timer = time.AfterFunc(w.config.DebounceDuration, w.flush)
}

func (w *DebouncedWatcher) flush() {
	w.mu.Lock()
	if w.stopped || len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}

	files := make([]string, 0, len(w.pending))
	for f := range w.pending {
		files = append(files, f)
	}
	w.pending = make(map[string]struct{})
	parallelFiles := w.config.ParallelFiles
	w.mu.Unlock()

	// Process files with parallelism limit
	if parallelFiles <= 1 || len(files) <= parallelFiles {
		w.onTrigger(files)
		return
	}

	// Process in batches for parallel execution
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallelFiles)

	for _, f := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func(file string) {
			defer wg.Done()
			defer func() { <-sem }()
			w.onTrigger([]string{file})
		}(f)
	}
	wg.Wait()
}

// Stop gracefully shuts down the watcher
func (w *DebouncedWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stopped {
		return
	}

	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
	close(w.stopCh)
}

// ShouldWatch checks if a file should be watched based on patterns
func (w *DebouncedWatcher) ShouldWatch(path string) bool {
	w.mu.Lock()
	config := w.config
	w.mu.Unlock()

	return ShouldWatchPath(path, config.WatchPatterns, config.IgnorePatterns)
}

// ShouldWatchPath checks if a path matches watch patterns and doesn't match ignore patterns
func ShouldWatchPath(path string, watchPatterns, ignorePatterns []string) bool {
	// Normalize path for pattern matching
	normalizedPath := filepath.ToSlash(path)
	// Strip file:// prefix if present
	normalizedPath = strings.TrimPrefix(normalizedPath, "file://")

	// Check ignore patterns first
	for _, pattern := range ignorePatterns {
		if matchGlobPattern(normalizedPath, pattern) {
			return false
		}
	}

	// If no watch patterns specified, watch everything not ignored
	if len(watchPatterns) == 0 {
		return true
	}

	// Check if path matches any watch pattern
	for _, pattern := range watchPatterns {
		if matchGlobPattern(normalizedPath, pattern) {
			return true
		}
	}

	return false
}

// matchGlobPattern matches a path against a glob pattern
// Supports ** for recursive matching and * for single-level matching
func matchGlobPattern(path, pattern string) bool {
	// Simple implementation for common glob patterns
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// Handle **/<dir>/** patterns (e.g., "**/node_modules/**", "**/vendor/**")
	if strings.HasPrefix(pattern, "**/") && strings.HasSuffix(pattern, "/**") {
		// Extract the directory name to match
		dirPattern := strings.TrimPrefix(pattern, "**/")
		dirPattern = strings.TrimSuffix(dirPattern, "/**")

		// Check if the directory appears anywhere in the path
		// Match /node_modules/ or /vendor/ etc.
		searchPattern := "/" + dirPattern + "/"
		return strings.Contains(path, searchPattern)
	}

	// Handle ** recursive patterns with suffix (e.g., "**/*.go")
	if strings.Contains(pattern, "**") {
		// Split pattern by **
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.TrimSuffix(parts[0], "/")
			suffix := strings.TrimPrefix(parts[1], "/")

			// Check if prefix matches start of path (or is empty)
			if prefix != "" && !strings.HasPrefix(path, prefix) {
				return false
			}

			// Check if suffix matches any part of remaining path
			if suffix != "" {
				// For suffix patterns like "*.go", check against filename
				if strings.HasPrefix(suffix, "*") {
					ext := strings.TrimPrefix(suffix, "*")
					return strings.HasSuffix(path, ext)
				}
				return strings.Contains(path, suffix) || strings.HasSuffix(path, suffix)
			}

			return true
		}
	}

	// Handle simple extension patterns like "*.go"
	if strings.HasPrefix(pattern, "*") && !strings.Contains(pattern, "/") {
		ext := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(path, ext)
	}

	// Fall back to filepath.Match for simple patterns
	matched, _ := filepath.Match(pattern, filepath.Base(path))
	return matched
}

// ParseDuration parses a duration string like "5m", "300ms", etc.
func ParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}
