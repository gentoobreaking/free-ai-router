//go:build unix

package cli

import "syscall"

func syscallExec(exe string, args []string) error {
	return syscall.Exec(exe, append([]string{exe}, args...), syscall.Environ())
}
