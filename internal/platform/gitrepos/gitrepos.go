package gitrepos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"reasonix/internal/platform/gitcmd"
)

const (
	// MaxDepth is how far below the workspace a repository may sit: 1 is a
	// direct child, 2 a grandchild.
	MaxDepth = 2
	// MaxRepos bounds the repositories kept.
	MaxRepos = 24
	// maxDirs bounds the directories read while looking.
	maxDirs = 400
)

// Found is a repository inside a workspace. Dir is its path relative to the
// workspace, slash-separated; empty when the workspace is itself the repository.
type Found struct {
	Dir  string
	Repo gitcmd.Repo
}

// Result is what a search settled. Truncated is true when a bound ended the
// search before every directory was read, so the list may be incomplete.
type Result struct {
	Repos     []Found
	Truncated bool
	// What a Finder compares to know the answer still holds: the directories whose
	// entries decided it (by modification time), and each repository's .git (by
	// being there). Work inside a repository changes neither.
	watch   []string
	present []string
}

// Find is the repository listed under dir, the folder below the workspace as
// Repos names it, or ErrNotListed. Nothing is built from dir: it is matched
// against what the search found.
func (r Result) Find(dir string) (gitcmd.Repo, error) {
	for _, f := range r.Repos {
		if f.Dir == dir {
			return f.Repo, nil
		}
	}
	return gitcmd.Repo{}, ErrNotListed
}

// Discover lists the repositories of the workspace at root. A workspace that is
// a repository is just that one: nothing inside it is looked for. Otherwise the
// directories below it are read breadth first to MaxDepth, a directory that is
// the top of its own work tree is kept and not entered, and symbolic links are
// not followed, so nothing outside the workspace is read.
func Discover(ctx context.Context, root string) (Result, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	if repo, err := gitcmd.Open(ctx, abs); err == nil {
		return Result{Repos: []Found{{Repo: repo}}, watch: []string{abs}, present: []string{filepath.Join(abs, ".git")}}, nil
	} else if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	var out Result
	type level struct {
		dir   string
		depth int
	}
	queue := []level{{abs, 0}}
	read := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= MaxDepth {
			continue
		}
		out.watch = append(out.watch, cur.dir)
		entries, err := os.ReadDir(cur.dir)
		if err != nil {
			continue
		}
		read++
		if read > maxDirs {
			out.Truncated = true
			break
		}
		for _, e := range entries {
			if !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			child := filepath.Join(cur.dir, e.Name())
			if repo, ok := topOfItsOwnTree(ctx, child); ok {
				out.present = append(out.present, filepath.Join(child, ".git"))
				if len(out.Repos) >= MaxRepos {
					out.Truncated = true
					continue
				}
				rel, _ := filepath.Rel(abs, child)
				out.Repos = append(out.Repos, Found{Dir: filepath.ToSlash(rel), Repo: repo})
				continue
			}
			out.watch = append(out.watch, child)
			queue = append(queue, level{child, cur.depth + 1})
		}
	}
	sort.Slice(out.Repos, func(i, j int) bool { return out.Repos[i].Dir < out.Repos[j].Dir })
	return out, nil
}

// topOfItsOwnTree opens dir and keeps it only when the work tree git resolved is
// dir itself: a folder inside some other repository resolves to that one and is
// not a repository of its own.
func topOfItsOwnTree(ctx context.Context, dir string) (gitcmd.Repo, bool) {
	if _, err := os.Lstat(filepath.Join(dir, ".git")); err != nil {
		return gitcmd.Repo{}, false
	}
	repo, err := gitcmd.Open(ctx, dir)
	if err != nil {
		return gitcmd.Repo{}, false
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || filepath.Clean(real) != repo.WorkTree {
		return gitcmd.Repo{}, false
	}
	return repo, true
}

// ErrNotListed is returned for a repository name the search did not find: outside
// the workspace, deeper than MaxDepth, behind a link, not the top of its own work
// tree, or named when the workspace is itself a repository.
var ErrNotListed = errors.New("gitrepos: not a repository of this workspace")
