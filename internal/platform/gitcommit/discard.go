package gitcommit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/platform/gitcmd"
	"reasonix/internal/platform/gitstatus"
)

// ErrNotDiscardable is a change that is not a modification of a file the last
// commit holds: a new file, a rename or an unmerged path has no earlier content
// to go back to, and removing it would be deleting something git never recorded.
var ErrNotDiscardable = errors.New("gitcommit: only a change to a file the last commit holds can be discarded")

// Discard puts one file back as the last commit has it, in the index and in the
// work tree, and the change is gone: nothing here keeps a copy. Only a path git
// itself lists as changed is accepted, and only a modified or deleted file that
// HEAD holds.
func Discard(ctx context.Context, repo gitcmd.Repo, path string) error {
	changes, ok, err := gitstatus.Status(ctx, repo)
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if !ok {
		return ErrNoRepository
	}
	var change *gitstatus.Change
	for i := range changes {
		if changes[i].Path == path {
			change = &changes[i]
			break
		}
	}
	if change == nil || isDirectory(repo.Dir, change.Path) {
		return fmt.Errorf("%w: %q", ErrUnknownPath, path)
	}
	if change.OldPath != "" || !onlyModifyOrDelete(change.Status) {
		return ErrNotDiscardable
	}
	if repo.Command(ctx, "rev-parse", "--verify", "--quiet", "HEAD").Run() != nil {
		return ErrNotDiscardable
	}
	var input bytes.Buffer
	input.WriteString(":(literal)" + path + "\x00")
	cmd := repo.Command(ctx, "restore", "--source=HEAD", "--staged", "--worktree", "--pathspec-from-file=-", "--pathspec-file-nul")
	cmd.Stdin = &input
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git restore: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// onlyModifyOrDelete is a status made of the letters for a changed or removed
// file and nothing else, in either column.
func onlyModifyOrDelete(status string) bool {
	for _, r := range status {
		if r != ' ' && r != 'M' && r != 'D' && r != 'T' {
			return false
		}
	}
	return strings.TrimSpace(status) != ""
}
