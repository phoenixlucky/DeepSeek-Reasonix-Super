//go:build !windows

package workspacefile

import "syscall"

const nonblock = syscall.O_NONBLOCK
