package testenv

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// removalRaceErrors names the errors a directory being deleted produces while
// it can still be stat'ed; each platform file appends its own.
var removalRaceErrors []error

// VanishedMidWalk reports whether a walk error means the entry was removed
// under the walker, as another package's test removing its in-tree folder does.
// A removal-race error counts only when the path is gone on a re-stat, so an
// unreadable directory that still exists keeps failing.
func VanishedMidWalk(path string, err error) bool {
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	racing := false
	for _, e := range removalRaceErrors {
		if errors.Is(err, e) {
			racing = true
		}
	}
	if !racing {
		return false
	}
	for range 500 {
		if _, serr := os.Lstat(path); errors.Is(serr, fs.ErrNotExist) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
