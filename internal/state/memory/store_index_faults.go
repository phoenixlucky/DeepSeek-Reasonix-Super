package memory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ErrIndexNotRegular identifies a MEMORY.md entry that exists but is not a
// regular file: a directory or special file cannot be read as text, and the
// fault is a property of the entry itself, not of the moment.
var ErrIndexNotRegular = errors.New("memory index is not a regular file")

// deterministicIndexFault reports whether an index read failure is a property
// of the file, which a rebuild can repair, rather than a transient error that
// may leave the existing bytes perfectly good.
func deterministicIndexFault(err error) bool {
	return errors.Is(err, fs.ErrPermission) || errors.Is(err, ErrIndexNotRegular)
}

// moveIndexAside keeps the unreadable entry under a stamped name in the same
// directory. A rename needs write access to the directory, not read access to
// the file, so it preserves bytes for exactly the faults that reach a rebuild;
// the new name does not end in .md, so the store never reads it back.
func moveIndexAside(dir string) error {
	path := filepath.Join(dir, indexFile)
	if err := fault("rename", path); err != nil {
		return err
	}
	aside := fmt.Sprintf("%s.unreadable-%d", indexFile, time.Now().UnixNano())
	return os.Rename(path, filepath.Join(dir, aside))
}

// repairIndexIn restricts the rebuild to the deterministic faults: every other
// read error returns as before, leaving the file untouched.
func (s Store) repairIndexIn(dir string, entries []os.DirEntry, cause error) error {
	if !deterministicIndexFault(cause) {
		return cause
	}
	return s.rebuildIndexIn(dir, entries, cause)
}
