package gitpush

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"reasonix/internal/platform/gitcmd"
)

var (
	// ErrDetachedHead is a repository on no branch: there is nothing to name.
	ErrDetachedHead = errors.New("gitpush: HEAD is not on a branch")
	// ErrNoUpstream is a branch that has no remote branch recorded for it.
	ErrNoUpstream = errors.New("gitpush: the branch has no upstream")
	// ErrNothingToPush is an upstream that already has every commit.
	ErrNothingToPush = errors.New("gitpush: the upstream already has every commit")
)

// Plan is what a push would do, read without touching the network. It is what
// the person confirms: Push refuses to run when the repository no longer matches.
type Plan struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`
	// UpstreamBranch is the branch the push updates on the remote.
	UpstreamBranch string `json:"upstreamBranch"`
	Scheme         string `json:"scheme"`
	Host           string `json:"host"`
	// URL is the remote as it will be reached, after the person's own url
	// rewrites and without any password in it.
	URL string `json:"url"`
	// Head is the commit the branch is at; it is exactly what is sent.
	Head string `json:"head"`
	// Ahead counts commits the remote's branches do not have. Behind counts those
	// the last known remote state has and this branch does not; it is only as
	// fresh as the last fetch.
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
}

// Preview works out what pushing the current branch would send and where, from
// the repository's recorded state alone. It reaches no remote and writes nothing
// the repository keeps.
func Preview(ctx context.Context, repo gitcmd.Repo) (Plan, error) {
	p, _, err := preview(ctx, repo)
	return p, err
}

// preview also returns the destination's raw URL, which Push needs and the
// caller of Preview does not.
func preview(ctx context.Context, repo gitcmd.Repo) (Plan, string, error) {
	if !repo.Valid() {
		return Plan{}, "", gitcmd.ErrNotRepository
	}
	top := repo.Top()
	branch, err := output(ctx, top, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// With --quiet git answers a detached HEAD with exit status 1 and says
		// nothing; any other failure is git not working, which is not a detached HEAD.
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return Plan{}, "", ErrDetachedHead
		}
		return Plan{}, "", fmt.Errorf("gitpush: read HEAD: %w", err)
	}
	if branch == "" {
		return Plan{}, "", ErrDetachedHead
	}
	remote, _ := configValue(ctx, top, "branch."+branch+".remote")
	merge, _ := configValue(ctx, top, "branch."+branch+".merge")
	upstream, isBranch := strings.CutPrefix(merge, "refs/heads/")
	if remote == "" || remote == "." || merge == "" || !isBranch || upstream == "" {
		return Plan{}, "", ErrNoUpstream
	}
	raw, _ := configValue(ctx, top, "remote."+remote+".pushurl")
	if raw == "" {
		raw, _ = configValue(ctx, top, "remote."+remote+".url")
	}
	if raw == "" {
		return Plan{}, "", ErrNoUpstream
	}
	head, err := output(ctx, top, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err != nil || head == "" {
		return Plan{}, "", ErrDetachedHead
	}
	effective, err := effectiveURL(ctx, repo, raw)
	if err != nil {
		return Plan{}, "", err
	}
	dest, err := Classify(effective)
	if err != nil {
		return Plan{}, "", err
	}
	plan := Plan{Remote: remote, Branch: branch, UpstreamBranch: upstream, Scheme: dest.Scheme, Host: dest.Host, URL: dest.Display, Head: head}
	if n, err := output(ctx, top, "rev-list", "--count", "refs/heads/"+branch, "--not", "--remotes="+remote); err == nil {
		plan.Ahead, _ = strconv.Atoi(n)
	}
	if tracking := "refs/remotes/" + remote + "/" + upstream; top.Command(ctx, "rev-parse", "--verify", "--quiet", tracking).Run() == nil {
		if n, err := output(ctx, top, "rev-list", "--count", tracking, "--not", "refs/heads/"+branch); err == nil {
			plan.Behind, _ = strconv.Atoi(n)
		}
	}
	return plan, raw, nil
}

func output(ctx context.Context, repo gitcmd.Repo, args ...string) (string, error) {
	out, err := repo.Command(ctx, args...).Output()
	return strings.TrimSpace(string(out)), err
}

// configValue reads one key as the repository records it, with no rewriting.
func configValue(ctx context.Context, repo gitcmd.Repo, key string) (string, error) {
	return output(ctx, repo, "config", "--get", "--", key)
}

// effectiveURL is the remote after the person's own url.<base>.insteadOf and
// pushInsteadOf rules. It is read from a throwaway repository, so a rewrite the
// repository itself recorded is not applied and not shown.
func effectiveURL(ctx context.Context, repo gitcmd.Repo, raw string) (string, error) {
	scratch, cleanup, err := newScratch(ctx, repo)
	if err != nil {
		return "", err
	}
	defer cleanup()
	out, err := scratch.repo.Command(ctx, "ls-remote", "--get-url", "--", raw).Output()
	if err != nil {
		return "", fmt.Errorf("gitpush: resolve remote: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// scratch is a repository with no configuration of its own whose objects are the
// real repository's. A push from it sends the same commits and nothing of the
// repository's .git/config comes along.
type scratch struct {
	repo gitcmd.Repo
	dir  string
}

func newScratch(ctx context.Context, real gitcmd.Repo) (*scratch, func(), error) {
	dir, err := os.MkdirTemp("", "reasonix-push-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if out, err := gitcmd.Command(ctx, "", "init", "-q", "--", dir).CombinedOutput(); err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("gitpush: scratch repository: %w: %s", err, strings.TrimSpace(string(out)))
	}
	objects := filepath.Join(real.CommonDir, "objects")
	alternates := filepath.Join(dir, ".git", "objects", "info", "alternates")
	if err := os.MkdirAll(filepath.Dir(alternates), 0o700); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if err := os.WriteFile(alternates, []byte(objects+"\n"), 0o600); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	opened, err := gitcmd.Open(ctx, dir)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return &scratch{repo: opened, dir: dir}, cleanup, nil
}
