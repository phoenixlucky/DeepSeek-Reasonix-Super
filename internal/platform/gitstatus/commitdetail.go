package gitstatus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"reasonix/internal/platform/gitcmd"
)

// ErrBadCommit rejects text that is not a commit id: an id is 7 to 64 hex digits,
// which is also what keeps a caller's text from ever being read as an option.
var ErrBadCommit = errors.New("gitstatus: not a commit id")

// ErrNoSuchCommit is a well-formed id that names no commit of this repository.
var ErrNoSuchCommit = errors.New("gitstatus: no such commit")

var commitID = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

// CommitFile is one path a commit changed. OldPath is set for a rename or copy.
// Absent counts print nothing: git did not count a binary file, which is not zero.
type CommitFile struct {
	Path       string `json:"path"`
	OldPath    string `json:"oldPath,omitempty"`
	Status     string `json:"status"`
	Insertions *int   `json:"insertions,omitempty"`
	Deletions  *int   `json:"deletions,omitempty"`
}

// MaxCommitFiles bounds the files listed for one commit.
const MaxCommitFiles = 500

// CommitFiles lists what one commit changed against its first parent, or against
// the empty tree for a root commit. ok is false for a workspace that is not
// version controlled. Nothing is written and no hook, textconv or external diff
// driver runs.
func CommitFiles(ctx context.Context, repo gitcmd.Repo, id string) (files []CommitFile, truncated bool, ok bool, err error) {
	if !repo.Valid() {
		return nil, false, false, nil
	}
	base, err := commitBase(ctx, repo, id)
	if err != nil {
		return nil, false, true, err
	}
	top := repo.Top()
	raw, err := top.Command(ctx, "diff", "--no-color", "-M", "-z", "--name-status", base, id, "--").Output()
	if err != nil {
		return nil, false, true, err
	}
	counts, _ := top.Command(ctx, "diff", "--no-color", "-M", "-z", "--numstat", base, id, "--").Output()
	stat := ParseNumstatZ(counts)
	files = []CommitFile{}
	parts := bytes.Split(raw, []byte{0})
	for i := 0; i < len(parts); i++ {
		status := string(parts[i])
		if status == "" {
			continue
		}
		f := CommitFile{Status: status[:1]}
		switch status[0] {
		case 'R', 'C':
			if i+2 >= len(parts) {
				return files, truncated, true, nil
			}
			f.OldPath, f.Path = string(parts[i+1]), string(parts[i+2])
			i += 2
		default:
			if i+1 >= len(parts) {
				return files, truncated, true, nil
			}
			f.Path = string(parts[i+1])
			i++
		}
		if n, found := stat[f.Path]; found {
			f.Insertions, f.Deletions = n.added, n.removed
		}
		if len(files) >= MaxCommitFiles {
			truncated = true
			break
		}
		files = append(files, f)
	}
	return files, truncated, true, nil
}

// CommitDiff is one path's diff in one commit, against the same base CommitFiles
// uses, capped at MaxDiffBytes.
func CommitDiff(ctx context.Context, repo gitcmd.Repo, id, path string) (text string, truncated bool, err error) {
	rel, err := safeRel(repo.Top().Dir, path)
	if err != nil {
		return "", false, err
	}
	base, err := commitBase(ctx, repo, id)
	if err != nil {
		return "", false, err
	}
	raw, err := repo.Top().Command(ctx, "diff", "--no-color", "-M", base, id, "--", rel).Output()
	if err != nil {
		return "", false, err
	}
	if len(raw) > MaxDiffBytes {
		return string(raw[:MaxDiffBytes]), true, nil
	}
	return string(raw), false, nil
}

// commitBase is what a commit is measured against: its first parent, or the empty
// tree when it has none. The id is checked to be a commit before anything else
// runs with it.
func commitBase(ctx context.Context, repo gitcmd.Repo, id string) (string, error) {
	if !commitID.MatchString(id) {
		return "", ErrBadCommit
	}
	top := repo.Top()
	if err := top.Command(ctx, "rev-parse", "--verify", "--quiet", id+"^{commit}").Run(); err != nil {
		// With --quiet git answers an id that names no commit with exit status 1 and
		// nothing else; any other failure is git not working, which says nothing
		// about the commit.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", ErrNoSuchCommit
		}
		return "", fmt.Errorf("gitstatus: resolve commit: %w", err)
	}
	if out, err := top.Command(ctx, "rev-parse", "--verify", "--quiet", id+"^1").Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	empty, err := top.Command(ctx, "hash-object", "-t", "tree", "--", os.DevNull).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(empty)), nil
}
