package output

import (
	"io"
	"log/slog"
	"math"
)

// SetupLogger creates a slog.Logger configured for the given verbosity.
// Output is written to w (typically os.Stderr).
//
// Log level mapping:
//   - quiet=true: Suppress ALL output (level set to math.MaxInt to disable all messages)
//   - debug=true: slog.LevelDebug
//   - verbose=true: slog.LevelInfo
//   - Default (all false): slog.LevelWarn (only warnings and errors)
//
// Priority: quiet > debug > verbose > default
func SetupLogger(quiet, verbose, debug bool, w io.Writer) *slog.Logger {
	var level slog.Level

	switch {
	case quiet:
		level = slog.Level(math.MaxInt)
	case debug:
		level = slog.LevelDebug
	case verbose:
		level = slog.LevelInfo
	default:
		level = slog.LevelWarn
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler)
}
