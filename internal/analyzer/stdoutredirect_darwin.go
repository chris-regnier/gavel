//go:build darwin

package analyzer

import "syscall"

// dup2 duplicates oldfd onto newfd with POSIX dup2 semantics.
// Darwin's syscall package exposes Dup2 directly on both amd64 and arm64.
func dup2(oldfd, newfd int) error {
	return syscall.Dup2(oldfd, newfd)
}
