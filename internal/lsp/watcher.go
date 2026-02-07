// internal/lsp/watcher.go
package lsp

import (
	"sync"
	"time"
)

// DebouncedWatcher batches file changes and triggers analysis after quiet period
type DebouncedWatcher struct {
	debounce  time.Duration
	onTrigger func(files []string)

	mu      sync.Mutex
	pending map[string]struct{}
	timer   *time.Timer
	stopCh  chan struct{}
	stopped bool
}

func NewDebouncedWatcher(debounce time.Duration, onTrigger func(files []string)) *DebouncedWatcher {
	return &DebouncedWatcher{
		debounce:  debounce,
		onTrigger: onTrigger,
		pending:   make(map[string]struct{}),
		stopCh:    make(chan struct{}),
	}
}

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

	w.timer = time.AfterFunc(w.debounce, w.flush)
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
	w.mu.Unlock()

	w.onTrigger(files)
}

func (w *DebouncedWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
	}
	close(w.stopCh)
}
