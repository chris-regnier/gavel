//go:build linux || darwin

package analyzer

import (
	"log/slog"
	"sync"
	"syscall"
)

var stdoutMu sync.Mutex

// redirectStdoutToStderr swaps fd 1 to point at fd 2 (stderr) at the OS level.
// This is needed because BAML's Rust FFI runtime writes log lines directly to fd 1,
// bypassing Go's os.Stdout — so Go-level redirection is insufficient.
// The caller must call the returned restore function exactly once, typically via defer.
// The mutex is held for the caller's entire critical section (between redirect and restore)
// to serialize concurrent access to the process-global fd table.
// On error, returns a no-op restore — the caller should proceed without redirect.
func redirectStdoutToStderr() (func(), error) {
	noop := func() {}

	stdoutMu.Lock()

	savedFd, err := syscall.Dup(1)
	if err != nil {
		stdoutMu.Unlock()
		slog.Warn("failed to dup stdout for BAML redirect", "error", err)
		return noop, err
	}
	syscall.CloseOnExec(savedFd)

	if err := dup2(2, 1); err != nil {
		syscall.Close(savedFd)
		stdoutMu.Unlock()
		slog.Warn("failed to dup2 stderr onto stdout for BAML redirect", "error", err)
		return noop, err
	}

	return func() {
		if err := dup2(savedFd, 1); err != nil {
			slog.Warn("failed to restore stdout after BAML redirect", "error", err)
		}
		if err := syscall.Close(savedFd); err != nil {
			slog.Warn("failed to close saved stdout fd", "error", err)
		}
		stdoutMu.Unlock()
	}, nil
}
