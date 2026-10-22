//go:build !windows

package peermail

import (
	"os"
	"syscall"
)

// ownedByUser reports whether the directory belongs to the current user.
func ownedByUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

func privateMode(info os.FileInfo) bool { return info.Mode().Perm()&0o077 == 0 }
