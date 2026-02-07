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
