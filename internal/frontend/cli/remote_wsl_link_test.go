package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/platform/remote"
	"reasonix/internal/platform/remote/attach"
	"reasonix/internal/platform/remote/sftpfs"
)

func TestRemoteWSLAddWritesAWSLMachineAndRefusesWhatWSLWouldReadAsAnOption(t *testing.T) {
	isolateCLIConfigHome(t)
	if code := remoteWSLCLI([]string{"add", "dev", "Ubuntu-24.04", "--workspace", "/home/me/app"}); code != 0 {
		t.Fatalf("add = %d", code)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := cfg.RemoteHost("dev")
	if !ok || e.HostKind() != config.RemoteKindWSL || e.Distro != "Ubuntu-24.04" || e.Workspace != "/home/me/app" || e.Host != "" || e.Target() != "wsl:Ubuntu-24.04" {
		t.Fatalf("saved = %+v, %v", e, ok)
	}
	for _, bad := range []string{"-d", "--exec", "a b", `a"b`, strings.Repeat("a", 65)} {
		if code := remoteWSLCLI([]string{"add", "x", bad}); code == 0 {
			t.Errorf("distro %q was accepted", bad)
		}
	}
	if code := remoteAddCLI([]string{"box", "ada@10.0.0.4"}); code != 0 {
		t.Fatalf("ssh add = %d", code)
	}
	if code := remoteWSLCLI([]string{"add", "box", "Ubuntu"}); code == 0 {
		t.Fatal("a WSL entry replaced an SSH machine of the same name")
	}
	if cfg, _ = config.Load(); func() bool {
		b, _ := cfg.RemoteHost("box")
		return b.HostKind() != config.RemoteKindSSH || b.Host != "10.0.0.4"
	}() {
		t.Fatal("the refused add changed the SSH machine")
	}
}

type failingLink struct{ started bool }

func (l *failingLink) Exec(context.Context, string) (remote.ExecResult, error) {
	return remote.ExecResult{}, errors.New("no exec")
}
func (l *failingLink) Files() (sftpfs.Files, error) { return nil, errors.New("no files") }
func (l *failingLink) Start(context.Context) error {
	l.started = true
	return errors.New("distribution would not start")
}
func (l *failingLink) Close() error                              { return nil }
func (l *failingLink) Subscribe(func(remote.StatusEvent)) func() { return func() {} }
func (l *failingLink) Forwards() attach.Forwards                 { return nil }
func (l *failingLink) Dirs(context.Context, string) (string, []sftpfs.Entry, error) {
	return "", nil, errors.New("no dirs")
}

// A WSL machine in the book is reached through the pool's WSL link and never
// through an SSH dial, from both test and connect.
func TestTestingAndConnectingAWSLMachineNeverDialsSSH(t *testing.T) {
	isolateCLIConfigHome(t)
	if code := remoteWSLCLI([]string{"add", "dev", "Ubuntu"}); code != 0 {
		t.Fatal(code)
	}
	link := &failingLink{}
	old := newWSLPool
	defer func() { newWSLPool = old }()
	newWSLPool = func(ctx context.Context, _, _ string) *attach.Pool {
		return attach.NewPool(ctx, attach.Options{
			Dial: func(string, attach.Prompts) (*remote.Client, error) {
				t.Error("an SSH dial was made for a WSL machine")
				return nil, errors.New("no")
			},
			DialWSL: func(config.RemoteHostEntry) (attach.Link, error) { return link, nil },
		})
	}
	if code := remoteTestCLI([]string{"dev"}, "test"); code != 1 || !link.started {
		t.Fatalf("remote test = %d, started = %v", code, link.started)
	}
	link.started = false
	if code := remoteConnectCLI([]string{"connect", "dev"}, "test"); code != 1 || !link.started {
		t.Fatalf("remote connect = %d, started = %v", code, link.started)
	}
}
