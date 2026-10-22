package gitrepos

import (
	"context"
	"os"
	"sync"
	"time"
)

// maxWatched bounds how many directories a cached answer is guarded by; a search
// that met more than this is not cached and is done again each time.
const maxWatched = 2000

type watch struct {
	exists bool
	mod    time.Time
}

// Finder answers Discover for one workspace and keeps the answer until the
// workspace's structure changes. The structure is the directories the search
// met: a repository appearing or going away changes the modification time of a
// directory the search read or listed, and one stat of each is far cheaper than
// asking git about every candidate again.
type Finder struct {
	mu      sync.Mutex
	root    string
	result  Result
	watched map[string]watch
	present map[string]bool
	valid   bool
	runs    int // how many times the search itself ran, for the tests
}

// Find is Discover through the cache. It returns the cached Result while every
// watched directory is as it was.
func (f *Finder) Find(ctx context.Context, root string) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.valid && f.root == root && f.unchanged() {
		return f.result, nil
	}
	f.runs++
	res, err := Discover(ctx, root)
	if err != nil {
		f.valid = false
		return Result{}, err
	}
	f.root, f.result, f.valid = root, res, false
	if len(res.watch)+len(res.present) > 0 && len(res.watch)+len(res.present) <= maxWatched {
		f.watched = snapshot(res.watch)
		f.present = map[string]bool{}
		for _, p := range res.present {
			f.present[p] = stamp(p).exists
		}
		f.valid = true
	}
	return res, nil
}

func (f *Finder) unchanged() bool {
	for p, was := range f.present {
		if stamp(p).exists != was {
			return false
		}
	}
	for p, was := range f.watched {
		now := stamp(p)
		if now.exists != was.exists || !now.mod.Equal(was.mod) {
			return false
		}
	}
	return true
}

func stamp(p string) watch {
	info, err := os.Lstat(p)
	if err != nil {
		return watch{}
	}
	return watch{exists: true, mod: info.ModTime()}
}

func snapshot(paths []string) map[string]watch {
	out := make(map[string]watch, len(paths))
	for _, p := range paths {
		out[p] = stamp(p)
	}
	return out
}
