//go:build linux

package crashreport

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// sameBinary reports whether pid runs this same executable. Liveness alone
// cannot answer that: a pid is recycled, so the number in a dump file's name can
// belong to an unrelated process that is alive now. Every uncertain answer is
// true, because a file kept by mistake costs less than a crash report discarded.
func sameBinary(pid int) bool {
	if pid <= 0 {
		return false
	}
	self, err := os.Executable()
	if err != nil || strings.TrimSpace(self) == "" {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	// The kernel's own record of the program: no argv[0] to be defeated by a
	// bare name, which a second instance started through PATH carries.
	target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil || strings.TrimSpace(target) == "" {
		return true
	}
	// A program replaced on disk while running is reported with this suffix.
	target = strings.TrimSuffix(target, " (deleted)")
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	return filepath.Clean(target) == filepath.Clean(self)
}
