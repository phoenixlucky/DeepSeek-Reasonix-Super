package workspacelease

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

func pathLeaseRoot(t *testing.T) string {
	t.Helper()
	root := testenv.TempDir(t)
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPathLeasesConflictByExtent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second []string
		conflict      bool
	}{
		{"disjoint", []string{"a.go"}, []string{"b.go"}, false},
		{"same", []string{"a.go"}, []string{"a.go"}, true},
		{"directory", []string{"pkg"}, []string{"pkg/a.go"}, true},
		{"directory reverse", []string{"pkg/a.go"}, []string{"pkg"}, true},
		{"sibling prefix", []string{"pkg"}, []string{"pkg2/a.go"}, false},
		{"move source", []string{"old.go", "new.go"}, []string{"old.go"}, true},
		{"move destination", []string{"old.go", "new.go"}, []string{"new.go"}, true},
		{"unknown writer", []string{"a.go"}, nil, true},
		{"unknown holder", nil, []string{"a.go"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, locks := pathLeaseRoot(t), testenv.TempDir(t)
			first, err := New(root, locks, nil)
			if err != nil {
				t.Fatal(err)
			}
			second, err := New(root, locks, nil)
			if err != nil {
				t.Fatal(err)
			}
			first.BeginRun()
			second.BeginRun()
			defer first.EndRun()
			defer second.EndRun()
			if err := first.AcquirePaths(context.Background(), tc.first); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			err = second.AcquirePaths(ctx, tc.second)
			if tc.conflict && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("want conflict, got %v", err)
			}
			if !tc.conflict && err != nil {
				t.Fatalf("disjoint paths blocked: %v", err)
			}
		})
	}
}

func TestPathLeaseRetainsEarlierWritesAndRefusesWidening(t *testing.T) {
	root, locks := pathLeaseRoot(t), testenv.TempDir(t)
	a, _ := New(root, locks, nil)
	b, _ := New(root, locks, nil)
	a.BeginRun()
	b.BeginRun()
	defer a.EndRun()
	defer b.EndRun()
	if err := a.AcquirePaths(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if err := b.AcquirePaths(context.Background(), []string{"b"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := a.AcquirePaths(ctx, []string{"b"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("widening: %v", err)
	}
	if err := b.AcquirePaths(ctx, []string{"a"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("earlier claim lost: %v", err)
	}
	if !a.State().Acquired {
		t.Fatal("cancelled widening lost original lease")
	}
}

func TestUnresolvablePathLeaseFallsBackToWholeWorkspace(t *testing.T) {
	root, locks := pathLeaseRoot(t), testenv.TempDir(t)
	a, _ := New(root, locks, nil)
	b, _ := New(root, locks, nil)
	a.BeginRun()
	b.BeginRun()
	defer a.EndRun()
	defer b.EndRun()
	if err := a.AcquirePaths(context.Background(), []string{"  "}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := b.AcquirePaths(ctx, []string{"inside"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a blank path narrowed scope: %v", err)
	}
}

// A path that resolves to somewhere outside the workspace (an extra writable
// root) is its own claim: it excludes writers of the same path and nothing else.
func TestAPathOutsideTheWorkspaceIsItsOwnClaim(t *testing.T) {
	root, locks, extra := pathLeaseRoot(t), testenv.TempDir(t), testenv.TempDir(t)
	attempt := func(ctx context.Context, o *Owner, paths ...string) error { return o.AcquirePaths(ctx, paths) }
	short := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 150*time.Millisecond)
	}
	setup := func(t *testing.T, opts ...Option) (*Owner, *Owner) {
		a, _ := New(root, locks, nil)
		b, _ := New(root, locks, nil, opts...)
		a.BeginRun()
		b.BeginRun()
		t.Cleanup(func() { a.EndRun(); b.EndRun() })
		return a, b
	}
	for name, opts := range map[string][]Option{"strict": nil, "optimistic": {WithoutWholeWorkspaceHold()}} {
		t.Run("an outside write does not take the workspace from a file holder, "+name, func(t *testing.T) {
			a, b := setup(t, opts...)
			if err := attempt(context.Background(), a, filepath.Join(root, "tmp", "report.md")); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := short()
			defer cancel()
			if err := attempt(ctx, b, filepath.Join(extra, "out.txt")); err != nil {
				t.Fatalf("a write outside the workspace waited on a file inside it: %v", err)
			}
		})
	}
	t.Run("two writers of the same outside file exclude each other", func(t *testing.T) {
		a, b := setup(t)
		file := filepath.Join(extra, "out.txt")
		if err := attempt(context.Background(), a, file); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := short()
		defer cancel()
		if err := attempt(ctx, b, file); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("the same outside file was granted twice: %v", err)
		}
	})
	t.Run("an outside folder excludes the files under it", func(t *testing.T) {
		a, b := setup(t)
		if err := attempt(context.Background(), a, extra); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := short()
		defer cancel()
		if err := attempt(ctx, b, filepath.Join(extra, "sub", "out.txt")); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("a file under a held outside folder was granted: %v", err)
		}
	})
	t.Run("an outside claim leaves the workspace to others", func(t *testing.T) {
		a, b := setup(t)
		if err := attempt(context.Background(), a, filepath.Join(extra, "out.txt")); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := short()
		defer cancel()
		if err := attempt(ctx, b, filepath.Join(root, "src", "x.go")); err != nil {
			t.Fatalf("an outside claim held the workspace: %v", err)
		}
	})
}

func TestPathLeaseExcludesLegacyWorkspaceLock(t *testing.T) {
	root, locks := pathLeaseRoot(t), testenv.TempDir(t)
	o, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.BeginRun()
	defer o.EndRun()
	if err := o.AcquirePaths(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	release, err := tryLockFile(o.lockPath)
	if err == nil {
		release()
		t.Fatal("legacy process could write through a live path lease")
	}
	if !errors.Is(err, errHeld) {
		t.Fatal(err)
	}
}

func TestCrossProcessPathLeaseReclaimedAfterCrash(t *testing.T) {
	root, locks := pathLeaseRoot(t), testenv.TempDir(t)
	ready := filepath.Join(testenv.TempDir(t), "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkspaceLeaseHelperProcess$")
	cmd.Env = append(os.Environ(), "REASONIX_WORKSPACE_LEASE_HELPER=1",
		"REASONIX_WORKSPACE_LEASE_ROOT="+root, "REASONIX_WORKSPACE_LEASE_DIR="+locks,
		"REASONIX_WORKSPACE_LEASE_READY="+ready, "REASONIX_WORKSPACE_LEASE_PATH=a.go")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not acquire path lease")
		}
		time.Sleep(20 * time.Millisecond)
	}
	o, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.BeginRun()
	defer o.EndRun()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := o.AcquirePaths(ctx, []string{"b.go"}); err != nil {
		t.Fatalf("disjoint cross-process path blocked: %v", err)
	}
	blocked, stop := context.WithTimeout(context.Background(), 150*time.Millisecond)
	if err := o.AcquirePaths(blocked, []string{"a.go"}); !errors.Is(err, ErrConflict) {
		stop()
		t.Fatalf("live holder ignored: %v", err)
	}
	stop()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = cmd.Process.Wait()
	reclaimed, finish := context.WithTimeout(context.Background(), 3*time.Second)
	defer finish()
	if err := o.AcquirePaths(reclaimed, []string{"a.go"}); err != nil {
		t.Fatalf("crashed holder not reclaimed: %v", err)
	}
}

func TestIncompleteStoredExtentRemainsConservative(t *testing.T) {
	root, locks := pathLeaseRoot(t), testenv.TempDir(t)
	a, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.BeginRun()
	b.BeginRun()
	defer a.EndRun()
	defer b.EndRun()
	if err := a.AcquirePaths(context.Background(), []string{"a.go"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.scope.record, []byte(`{"paths":[""]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := b.AcquirePaths(ctx, []string{"b.go"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("incomplete stored extent established scope: %v", err)
	}
}
