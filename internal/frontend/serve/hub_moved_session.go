package serve

import (
	"path/filepath"
	"runtime"
	"strings"

	"reasonix/internal/state/store"
)

// currentSessionPath maps a session path remembered before the state root moved
// onto the same file name in this project's session directory. It answers only
// for a clean absolute <state>/projects/<this project>/sessions/<transcript>
// path, touches no file, and leaves the verdict to resumeInto.
func currentSessionPath(root, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return path
	}
	dir := SessionDirFor(root)
	if dir == "" || filepath.Dir(path) == dir {
		return path
	}
	name := filepath.Base(path)
	sessions := filepath.Dir(path)
	if !store.IsSessionTranscriptName(name) ||
		!sameFolderName(filepath.Base(sessions), filepath.Base(dir)) ||
		!sameFolderName(filepath.Base(filepath.Dir(sessions)), filepath.Base(filepath.Dir(dir))) {
		return path
	}
	return filepath.Join(dir, name)
}

// sameFolderName compares folder names the way the file system does: Windows
// folds case (the project folder's name is built lower-cased there), elsewhere
// the names must match exactly.
func sameFolderName(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
