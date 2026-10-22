package memory

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"reasonix/internal/base/filelock"
	"reasonix/internal/contract/config"
)

// memoryLockWait bounds how long a mutation queues behind another process. A
// wedged peer fails the caller instead of hanging it; nothing has been written
// when the wait expires.
var memoryLockWait = 2 * time.Second

// ErrStoreLocked is the identity of a mutation that lost the cross-process lock
// race. The caller may retry once the other process has finished.
var ErrStoreLocked = errors.New("memory store locked by another process")

// StoreLockedError says which directory was still held when the wait expired.
// It unwraps to ErrStoreLocked so a caller can match either form.
type StoreLockedError struct {
	Dir    string
	Waited time.Duration
}

func (e *StoreLockedError) Error() string {
	return fmt.Sprintf("memory store %s is locked by another process (waited %s)", e.Dir, e.Waited)
}

func (e *StoreLockedError) Unwrap() error { return ErrStoreLocked }

// memoryLockRoot is the user lock root every memory store lock file lives
// under. Tests isolate it by isolating the user state (testenv.IsolateUserState).
func memoryLockRoot() string { return config.MemoryLockDir() }

// lockMutations takes one lock file per directory this store mutates. Keys are
// canonical absolute paths sorted before any lock is taken, so two stores whose
// Dir and GlobalDir are swapped request the same order and cannot deadlock.
// Both directories are locked when the store has both: a scope-crossing rename
// writes the source index too, and the in-process mutex above is store-wide.
func (s Store) lockMutations() (func(), error) {
	keys := s.mutationLockKeys()
	if len(keys) == 0 {
		return func() {}, nil
	}
	root := strings.TrimSpace(memoryLockRoot())
	if root == "" {
		return nil, fmt.Errorf("memory store lock: no lock directory is available")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("memory store lock: create lock directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), memoryLockWait)
	defer cancel()
	releases := make([]func(), 0, len(keys))
	for _, key := range keys {
		release, err := filelock.Acquire(ctx, memoryLockPath(root, key))
		if err != nil {
			releaseAll(releases)
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, &StoreLockedError{Dir: key, Waited: memoryLockWait}
			}
			return nil, fmt.Errorf("memory store lock: %w", err)
		}
		releases = append(releases, release)
	}
	var once sync.Once
	return func() { once.Do(func() { releaseAll(releases) }) }, nil
}

// mutationLockKeys is the sorted, de-duplicated set of directories a mutation
// writes: this store's Dir and GlobalDir.
func (s Store) mutationLockKeys() []string {
	seen := make(map[string]struct{}, 2)
	keys := make([]string, 0, 2)
	for _, dir := range []string{s.Dir, s.GlobalDir} {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		key := canonicalLockDir(dir)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// canonicalLockDir folds path aliases so both processes on one directory agree
// on a key. A leaf that does not exist yet (the first save creates it) resolves
// through its parent.
func canonicalLockDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	abs = filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if resolved, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(resolved, filepath.Base(abs))
	}
	if runtime.GOOS == "windows" {
		abs = strings.ToLower(filepath.ToSlash(abs))
	}
	return abs
}

// memoryLockPath is the lock file named by the digest of key, so the file name
// is stable across processes and independent of what the path spells.
func memoryLockPath(root, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(root, fmt.Sprintf("%x.lock", sum))
}

func releaseAll(releases []func()) {
	for _, release := range slices.Backward(releases) {
		release()
	}
}
