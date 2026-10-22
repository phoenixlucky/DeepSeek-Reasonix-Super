//go:build !linux

package crashreport

// sameBinary cannot be answered off Linux, so it answers yes: the only platform
// where the answer changes is the one with a /proc to read.
func sameBinary(pid int) bool { return pid > 0 }
