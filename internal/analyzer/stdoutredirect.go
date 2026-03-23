//go:build linux || darwin

package analyzer

import (
	"log/slog"
	"sync"
	"syscall"
)

var stdoutMu sync.Mutex

// redirectStdoutToStderr swaps fd 1 to point at fd 2 (stderr).
// Returns a restore function and any error.
// On error, returns a no-op restore — the caller should proceed without redirect.
func redirectStdoutToStderr() (restore func(), err error) {
	noop := func() {}

	stdoutMu.Lock()

	savedFd, err := syscall.Dup(1)
	if err != nil {
		stdoutMu.Unlock()
		slog.Warn("failed to dup stdout for BAML redirect", "error", err)
		return noop, err
	}
	syscall.CloseOnExec(savedFd)

	if err := syscall.Dup2(2, 1); err != nil {
		syscall.Close(savedFd)
		stdoutMu.Unlock()
		slog.Warn("failed to dup2 stderr onto stdout for BAML redirect", "error", err)
		return noop, err
	}

	return func() {
		if err := syscall.Dup2(savedFd, 1); err != nil {
			slog.Warn("failed to restore stdout after BAML redirect", "error", err)
		}
		syscall.Close(savedFd)
		stdoutMu.Unlock()
	}, nil
}
