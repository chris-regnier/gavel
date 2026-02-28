package output

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestSetupLogger_DefaultLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(false, false, false, &buf)

	logger.Info("info message")
	if bytes.Contains(buf.Bytes(), []byte("info message")) {
		t.Error("expected Info message to be suppressed at default level (Warn), but it appeared in output")
	}

	buf.Reset()
	logger.Warn("warn message")
	if !bytes.Contains(buf.Bytes(), []byte("warn message")) {
		t.Error("expected Warn message to appear at default level, but it was suppressed")
	}
}

func TestSetupLogger_Quiet(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(true, false, false, &buf)

	logger.Warn("warn message")
	if bytes.Contains(buf.Bytes(), []byte("warn message")) {
		t.Error("expected Warn message to be suppressed in quiet mode, but it appeared in output")
	}

	buf.Reset()
	logger.Error("error message")
	if bytes.Contains(buf.Bytes(), []byte("error message")) {
		t.Error("expected Error message to be suppressed in quiet mode, but it appeared in output")
	}
}

func TestSetupLogger_Verbose(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(false, true, false, &buf)

	logger.Info("info message")
	if !bytes.Contains(buf.Bytes(), []byte("info message")) {
		t.Error("expected Info message to appear in verbose mode, but it was suppressed")
	}
}

func TestSetupLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(false, false, true, &buf)

	logger.Debug("debug message")
	if !bytes.Contains(buf.Bytes(), []byte("debug message")) {
		t.Error("expected Debug message to appear in debug mode, but it was suppressed")
	}
}

func TestSetupLogger_QuietOverridesDebug(t *testing.T) {
	var buf bytes.Buffer
	// quiet takes priority over debug
	logger := SetupLogger(true, false, true, &buf)

	logger.Error("error message")
	if bytes.Contains(buf.Bytes(), []byte("error message")) {
		t.Error("expected quiet to override debug, but Error message appeared in output")
	}
}

func TestSetupLogger_ReturnsValidLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(false, false, false, &buf)

	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	// Verify it's a *slog.Logger
	var _ *slog.Logger = logger
}
