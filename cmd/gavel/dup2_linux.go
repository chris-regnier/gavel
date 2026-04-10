//go:build linux

package main

import "golang.org/x/sys/unix"

// dup2 duplicates oldfd onto newfd with POSIX dup2 semantics.
// syscall.Dup2 is not exposed on linux/arm64 (the kernel only implements
// dup3 there), so this uses unix.Dup3 which is available on every linux arch.
func dup2(oldfd, newfd int) error {
	return unix.Dup3(oldfd, newfd, 0)
}
