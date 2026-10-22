package gitpush

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/platform/gitcmd"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fileURL spells a local path as the URL git expects on every platform: forward
// slashes and a leading one, so D:\x\y is file:///D:/x/y and a backslash never
// reaches a config value, where it would be read as an escape.
func fileURL(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

// local lets file:// stand in for a network remote; production allows none of it.
func local(t *testing.T) {
	t.Helper()
	old, oldSchemes := allowedProtocols, allowedSchemes
	allowedProtocols = "file:https:ssh:git"
	allowedSchemes = map[string]bool{"https": true, "ssh": true, "git": true, "file": true}
	t.Cleanup(func() { allowedProtocols, allowedSchemes = old, oldSchemes })
}

func isolate(t *testing.T) string {
	t.Helper()
	global := filepath.Join(testenv.TempDir(t), "gitconfig")
	write(t, filepath.Dir(global), "gitconfig", "")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(k+"_NAME", "t")
		t.Setenv(k+"_EMAIL", "t@t")
	}
	return global
}

type fixture struct {
	work, remote string
	repo         gitcmd.Repo
}

// newFixture is a work repository on main with an upstream bare repository it has
// pushed once, and one commit more locally.
func newFixture(t *testing.T) fixture {
	t.Helper()
	isolate(t)
	local(t)
	root := testenv.TempDir(t)
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	run(t, root, "init", "-q", "--bare", "-b", "main", remote)
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, work, "init", "-q", "-b", "main")
	write(t, work, "a.go", "package a\n")
	run(t, work, "add", ".")
	run(t, work, "commit", "-qm", "base")
	run(t, work, "remote", "add", "origin", fileURL(remote))
	run(t, work, "push", "-q", "-u", "origin", "main")
	write(t, work, "b.go", "package b\n")
	run(t, work, "add", ".")
	run(t, work, "commit", "-qm", "second")
	repo, err := gitcmd.Open(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{work: work, remote: remote, repo: repo}
}

func (f fixture) remoteHead(t *testing.T) string { return run(t, f.remote, "rev-parse", "main") }
func (f fixture) workHead(t *testing.T) string   { return run(t, f.work, "rev-parse", "HEAD") }

func TestPreviewNamesWhatWouldBeSentAndWhereWithoutTouchingTheNetwork(t *testing.T) {
	f := newFixture(t)
	plan, err := Preview(context.Background(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Remote != "origin" || plan.Branch != "main" || plan.UpstreamBranch != "main" || plan.Ahead != 1 || plan.Behind != 0 || plan.Head != f.workHead(t) {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Scheme != "file" || !strings.HasSuffix(plan.URL, "remote.git") {
		t.Fatalf("destination = %q %q", plan.Scheme, plan.URL)
	}
	if f.remoteHead(t) == f.workHead(t) {
		t.Fatal("previewing sent something")
	}
}

func TestPushSendsExactlyTheConfirmedCommitAndNoMore(t *testing.T) {
	f := newFixture(t)
	plan, _ := Preview(context.Background(), f.repo)
	res, err := Push(context.Background(), f.repo, plan)
	if err != nil {
		t.Fatal(err)
	}
	if res.New || f.remoteHead(t) != f.workHead(t) {
		t.Fatalf("remote at %s, work at %s, new=%v", f.remoteHead(t), f.workHead(t), res.New)
	}
	if got := run(t, f.work, "rev-parse", "refs/remotes/origin/main"); got != f.workHead(t) {
		t.Fatalf("tracking ref = %s, want the pushed commit", got)
	}
	again, _ := Preview(context.Background(), f.repo)
	if again.Ahead != 0 {
		t.Fatalf("ahead after pushing = %d", again.Ahead)
	}
	if _, err := Push(context.Background(), f.repo, again); !errors.Is(err, ErrNothingToPush) {
		t.Fatalf("pushing nothing = %v, want ErrNothingToPush", err)
	}
}

func TestNothingTheRepositoryConfiguresRunsOrRedirectsAPush(t *testing.T) {
	f := newFixture(t)
	marker := filepath.Join(testenv.TempDir(t), "ran")
	hook := filepath.Join(f.work, ".git", "hooks", "pre-push")
	write(t, filepath.Dir(hook), "pre-push", "#!/bin/sh\ntouch "+marker+"\n")
	if err := os.Chmod(hook, 0o700); err != nil {
		t.Fatal(err)
	}
	run(t, f.work, "config", "core.hooksPath", filepath.Join(f.work, ".git", "hooks"))
	run(t, f.work, "config", "core.sshCommand", "touch "+marker+" #")
	run(t, f.work, "config", "credential.helper", "!touch "+marker+" #")
	run(t, f.work, "config", "url.file:///nowhere/evil.insteadOf", fileURL(f.remote))
	plan, err := Preview(context.Background(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan.URL, "evil") {
		t.Fatalf("a rewrite the repository recorded was applied and shown: %q", plan.URL)
	}
	if _, err := Push(context.Background(), f.repo, plan); err != nil {
		t.Fatal(err)
	}
	if f.remoteHead(t) != f.workHead(t) {
		t.Fatal("the push did not reach the remote the person confirmed")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a program named by the repository's own configuration ran during the push")
	}
}

func TestTheUsersOwnRewriteIsAppliedAndShown(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(testenv.TempDir(t), "other.git")
	run(t, filepath.Dir(other), "init", "-q", "--bare", "-b", "main", other)
	global := os.Getenv("GIT_CONFIG_GLOBAL")
	write(t, filepath.Dir(global), filepath.Base(global), "[url \""+fileURL(other)+"\"]\n\tinsteadOf = "+fileURL(f.remote)+"\n")
	plan, err := Preview(context.Background(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(plan.URL, "other.git") {
		t.Fatalf("url = %q, want the user's rewrite shown, since that is where it goes", plan.URL)
	}
	res, err := Push(context.Background(), f.repo, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !res.New || run(t, other, "rev-parse", "main") != f.workHead(t) {
		t.Fatalf("the push did not go where the preview said (new=%v)", res.New)
	}
	if f.remoteHead(t) == f.workHead(t) {
		t.Fatal("the push also reached the unrewritten remote")
	}
}

func TestAPushThatIsNotAFastForwardIsRefusedAndNeverForced(t *testing.T) {
	f := newFixture(t)
	other := filepath.Join(testenv.TempDir(t), "other")
	run(t, filepath.Dir(other), "clone", "-q", fileURL(f.remote), other)
	write(t, other, "c.go", "package c\n")
	run(t, other, "add", ".")
	run(t, other, "commit", "-qm", "someone else")
	run(t, other, "push", "-q", "origin", "main")
	before := f.remoteHead(t)
	plan, _ := Preview(context.Background(), f.repo)
	if _, err := Push(context.Background(), f.repo, plan); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
	if f.remoteHead(t) != before {
		t.Fatal("a rejected push changed the remote: it was forced")
	}
	if got := run(t, f.work, "rev-parse", "refs/remotes/origin/main"); got == f.workHead(t) {
		t.Fatal("the tracking ref moved although nothing was pushed")
	}
}

func TestAPlanThatNoLongerMatchesIsRefused(t *testing.T) {
	f := newFixture(t)
	plan, _ := Preview(context.Background(), f.repo)
	write(t, f.work, "c.go", "package c\n")
	run(t, f.work, "add", ".")
	run(t, f.work, "commit", "-qm", "third, after the preview")
	if _, err := Push(context.Background(), f.repo, plan); !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("err = %v, want ErrPlanChanged", err)
	}
	if f.remoteHead(t) == f.workHead(t) {
		t.Fatal("a commit nobody confirmed was sent")
	}
}

func TestABranchWithNoUpstreamOrOnNoBranchHasNothingToPush(t *testing.T) {
	f := newFixture(t)
	run(t, f.work, "checkout", "-q", "-b", "topic")
	if _, err := Preview(context.Background(), f.repo); !errors.Is(err, ErrNoUpstream) {
		t.Fatalf("a branch with no upstream = %v, want ErrNoUpstream", err)
	}
	run(t, f.work, "checkout", "-q", "--detach", "HEAD")
	if _, err := Preview(context.Background(), f.repo); !errors.Is(err, ErrDetachedHead) {
		t.Fatalf("a detached head = %v, want ErrDetachedHead", err)
	}
}

func TestAProtocolOutsideTheAllowListIsRefusedBeforeAndByGitItself(t *testing.T) {
	f := newFixture(t)
	t.Cleanup(func() {
		allowedProtocols, allowedSchemes = "https:ssh:git", map[string]bool{"https": true, "ssh": true, "git": true}
	})
	allowedProtocols, allowedSchemes = "https:ssh:git", map[string]bool{"https": true, "ssh": true, "git": true}
	if _, err := Preview(context.Background(), f.repo); !errors.Is(err, ErrProtocolNotAllowed) {
		t.Fatalf("a file:// remote = %v, want ErrProtocolNotAllowed", err)
	}
	// With the pre-check opened, git's own environment still refuses the transport.
	plan := Plan{}
	plan.Remote, plan.Branch, plan.UpstreamBranch, plan.Head = "origin", "main", "main", f.workHead(t)
	allowedSchemes = map[string]bool{"https": true, "ssh": true, "git": true, "file": true}
	p, err := Preview(context.Background(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), f.repo, p); !errors.Is(err, ErrPushFailed) {
		t.Fatalf("err = %v, want git to refuse the transport (ErrPushFailed)", err)
	}
	if f.remoteHead(t) == f.workHead(t) {
		t.Fatal("a transport outside the allow-list carried a push")
	}
}

func TestAPushThatStoppedForACredentialIsToldFromOneThatFailed(t *testing.T) {
	f := newFixture(t)
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = true
		w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	allowedProtocols += ":http"
	allowedSchemes["http"] = true
	t.Cleanup(func() { delete(allowedSchemes, "http") })
	run(t, f.work, "remote", "set-url", "origin", srv.URL+"/r.git")
	plan, err := Preview(context.Background(), f.repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Push(context.Background(), f.repo, plan); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("err = %v (server asked: %v), want ErrAuthRequired", err, asked)
	}
}

func TestClassifyReadsTheDestinationByStructure(t *testing.T) {
	ok := map[string][3]string{
		"https://github.com/o/r.git":          {"https", "github.com", "https://github.com/o/r.git"},
		"https://user:secret@example.com/o/r": {"https", "example.com", "https://user@example.com/o/r"},
		"ssh://git@host.example:2222/o/r.git": {"ssh", "host.example", "ssh://git@host.example:2222/o/r.git"},
		"git://example.org/o/r.git":           {"git", "example.org", "git://example.org/o/r.git"},
		"git@github.com:o/r.git":              {"ssh", "github.com", "git@github.com:o/r.git"},
		"host.example:path/to/repo":           {"ssh", "host.example", "host.example:path/to/repo"},
		"HTTPS://Example.COM/o/r":             {"https", "Example.COM", "https://Example.COM/o/r"},
	}
	for raw, want := range ok {
		got, err := Classify(raw)
		if err != nil || got.Scheme != want[0] || got.Host != want[1] || got.Display != want[2] {
			t.Errorf("Classify(%q) = %+v, %v; want %v", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"", " ", "-oProxyCommand=x", "--upload-pack=x", "/srv/git/repo.git", "./repo", "../repo", "C:\\repo", "C:/repo",
		"file:///srv/repo", "ext::sh -c touch% /tmp/x", "fd::3", "transport::address", "http://example.com/r.git",
		"ftp://example.com/r.git", "https:///nohost", "ssh://", "https://\n", "git@:path", "javascript://x",
	} {
		if _, err := Classify(raw); !errors.Is(err, ErrProtocolNotAllowed) {
			t.Errorf("Classify(%q) err = %v, want ErrProtocolNotAllowed", raw, err)
		}
	}
}

func TestAGitThatCannotReadHeadIsNotADetachedHead(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.work, ".git"), "HEAD", "garbage that is not a ref\n")
	_, err := Preview(context.Background(), f.repo)
	if err == nil || errors.Is(err, ErrDetachedHead) {
		t.Fatalf("err = %v, want git's own failure and not ErrDetachedHead", err)
	}
}
