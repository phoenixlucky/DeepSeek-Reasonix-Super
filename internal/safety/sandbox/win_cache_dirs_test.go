package sandbox

import (
	"path/filepath"
	"strings"
	"testing"
)

// A granted root is labelled Low; a program run from a Low-labelled file runs at
// Low even outside the sandbox. So no listed directory may be, or contain, a
// place a person's own programs live.
func TestNoCacheDirIsOrContainsAPlaceProgramsAreRunFrom(t *testing.T) {
	local, home := filepath.Join("C:", "Users", "u", "AppData", "Local"), filepath.Join("C:", "Users", "u")
	within := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	dirs := cacheDirsFor(local, home)
	if len(dirs) == 0 {
		t.Fatal("no caches listed")
	}
	for _, cache := range dirs {
		for _, exe := range executableHomesFor(local, home) {
			if within(cache, exe) || within(exe, cache) {
				t.Errorf("cache %s overlaps %s, where programs are run from", cache, exe)
			}
		}
		if base := strings.ToLower(filepath.Base(cache)); base == "bin" || base == ".bin" || base == "tools" {
			t.Errorf("cache %s is named like a program folder", cache)
		}
	}
}

func TestCacheDirsNeedAnEnvironmentToBeListed(t *testing.T) {
	if got := cacheDirsFor("", ""); len(got) != 0 {
		t.Fatalf("with no profile there is nothing to list, got %v", got)
	}
}
