package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// DirectoryReadError identifies an incomplete store read and preserves its OS cause.
type DirectoryReadError struct {
	Path string
	Err  error
}

func (e *DirectoryReadError) Error() string {
	return fmt.Sprintf("read memory directory %q: %v", e.Path, e.Err)
}

func (e *DirectoryReadError) Unwrap() error { return e.Err }

func readMemoryDirectory(path string) ([]os.DirEntry, error) {
	if path == "" {
		return nil, nil
	}
	err := fault("read-dir", path)
	var entries []os.DirEntry
	if err == nil {
		// codeql[go/path-injection] path is a fixed subdirectory the host joins under the store root; it is only listed, never written.
		entries, err = os.ReadDir(path)
	}
	if os.IsNotExist(err) {
		// Windows answers "not found" for a regular file where a directory
		// should be, or beneath one; only the entry itself tells that from a missing path.
		for _, at := range []string{path, filepath.Dir(path)} {
			// codeql[go/path-injection] at is that same host-built directory or its parent, stat-ed for file kind only, never read or written.
			if info, statErr := os.Stat(at); statErr == nil && !info.IsDir() {
				return nil, &DirectoryReadError{Path: at, Err: err}
			}
		}
		return nil, nil
	}
	if err != nil {
		return nil, &DirectoryReadError{Path: path, Err: err}
	}
	return entries, nil
}
