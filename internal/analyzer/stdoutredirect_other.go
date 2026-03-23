//go:build !linux && !darwin

package analyzer

// redirectStdoutToStderr is a no-op on unsupported platforms.
func redirectStdoutToStderr() (func(), error) {
	return func() {}, nil
}
