//go:build linux || darwin

package analyzer

import (
	"io"
	"os"
	"syscall"
	"testing"
)

func TestRedirectStdoutToStderr(t *testing.T) {
	// Save original fds for unconditional cleanup
	origStdoutFd, err := syscall.Dup(1)
	if err != nil {
		t.Fatalf("failed to dup stdout: %v", err)
	}
	origStderrFd2, err := syscall.Dup(2)
	if err != nil {
		syscall.Close(origStdoutFd)
		t.Fatalf("failed to dup stderr for cleanup: %v", err)
	}
	t.Cleanup(func() {
		// Unconditionally restore real fds to protect the test binary
		dup2(origStdoutFd, 1)
		dup2(origStderrFd2, 2)
		syscall.Close(origStdoutFd)
		syscall.Close(origStderrFd2)
	})

	// Create a pipe to act as our "stdout" sink
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	defer stdoutR.Close()

	// Create a pipe to act as our "stderr" sink
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}
	defer stderrR.Close()

	// Point fd 1 (stdout) at our stdout pipe
	if err := dup2(int(stdoutW.Fd()), 1); err != nil {
		t.Fatalf("failed to redirect stdout to pipe: %v", err)
	}
	stdoutW.Close()

	// Point fd 2 (stderr) at our stderr pipe
	origStderrFd, err := syscall.Dup(2)
	if err != nil {
		t.Fatalf("failed to dup stderr: %v", err)
	}
	defer syscall.Close(origStderrFd)

	if err := dup2(int(stderrW.Fd()), 2); err != nil {
		t.Fatalf("failed to redirect stderr to pipe: %v", err)
	}
	stderrW.Close()

	// Phase 1: Write before redirect — should go to stdout pipe
	syscall.Write(1, []byte("before-redirect|"))

	// Phase 2: Activate redirect
	restore, redirectErr := redirectStdoutToStderr()
	if redirectErr != nil {
		t.Fatalf("redirectStdoutToStderr failed: %v", redirectErr)
	}

	// Write to fd 1 — should go to stderr pipe during redirect
	syscall.Write(1, []byte("during-redirect"))

	// Phase 3: Restore
	restore()

	// Write to fd 1 — should go to stdout pipe after restore
	syscall.Write(1, []byte("after-restore"))

	// Restore real fds before reading pipes — close write-ends by restoring originals
	dup2(origStdoutFd, 1)
	dup2(origStderrFd, 2)

	// Read all data from pipes
	stdoutData, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("failed to read stdout pipe: %v", err)
	}

	stderrData, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("failed to read stderr pipe: %v", err)
	}

	if string(stdoutData) != "before-redirect|after-restore" {
		t.Errorf("expected stdout to contain 'before-redirect|after-restore', got %q", string(stdoutData))
	}
	if string(stderrData) != "during-redirect" {
		t.Errorf("expected stderr to contain 'during-redirect', got %q", string(stderrData))
	}
}
