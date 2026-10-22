package workspacelease

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func named(o *Owner, name, id string) {
	o.SetHolder(func() string { return name })
	o.SetSessionID(func() string { return id })
}

func TestResumeNamesTheSessionThatClaimedTheParkedPathMeanwhile(t *testing.T) {
	a, b, root := yieldPair(t)
	named(a, "alpha", "sess-a")
	named(b, "beta", "sess-b")
	file := filepath.Join(root, "x.go")
	a.BeginRun()
	b.BeginRun()
	if err := a.AcquirePaths(context.Background(), []string{file}); err != nil {
		t.Fatal(err)
	}
	resume := a.Yield(context.Background())
	if err := b.AcquirePaths(context.Background(), []string{file}); err != nil {
		t.Fatal(err)
	}
	b.EndRun()
	got, err := resume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Unknown || len(got.Contenders) != 1 || got.Contenders[0].SessionID != "sess-b" || got.Contenders[0].Holder != "beta" {
		t.Fatalf("contention = %+v, want exactly session sess-b", got)
	}
}

func TestResumeReportsNothingWhenNobodyClaimedTheParkedPath(t *testing.T) {
	a, b, root := yieldPair(t)
	named(a, "alpha", "sess-a")
	named(b, "beta", "sess-b")
	a.BeginRun()
	b.BeginRun()
	if err := a.AcquirePaths(context.Background(), []string{filepath.Join(root, "x.go")}); err != nil {
		t.Fatal(err)
	}
	resume := a.Yield(context.Background())
	if err := b.AcquirePaths(context.Background(), []string{filepath.Join(root, "other.go")}); err != nil {
		t.Fatal(err)
	}
	got, err := resume(context.Background())
	if err != nil || got.Any() {
		t.Fatalf("resume = %+v, %v; a claim on another path is not contention", got, err)
	}
}

func TestResumeReportsNothingForAYieldNobodyUsed(t *testing.T) {
	a, _, root := yieldPair(t)
	named(a, "alpha", "sess-a")
	a.BeginRun()
	if err := a.AcquirePaths(context.Background(), []string{filepath.Join(root, "x.go")}); err != nil {
		t.Fatal(err)
	}
	got, err := a.Yield(context.Background())(context.Background())
	if err != nil || got.Any() {
		t.Fatalf("resume = %+v, %v; the session's own claim is not contention", got, err)
	}
}

func TestACallThatAsksBeforeWritingYieldsItsOwnHold(t *testing.T) {
	a, b, root := yieldPair(t)
	named(a, "alpha", "sess-a")
	named(b, "beta", "sess-b")
	file := filepath.Join(root, "x.go")
	a.BeginRun()
	b.BeginRun()
	ctx, end, err := a.HoldPaths(context.Background(), []string{file})
	if err != nil {
		t.Fatal(err)
	}
	defer end()
	if err := within(t, b, []string{file}, 200*time.Millisecond); err == nil {
		t.Fatal("b wrote a path a call is holding")
	}
	resume := a.Yield(ctx)
	if a.State().Acquired {
		t.Fatal("a call asking before it writes still holds the claim")
	}
	if err := b.AcquirePaths(context.Background(), []string{file}); err != nil {
		t.Fatalf("b blocked behind a call that was only asking: %v", err)
	}
	b.EndRun()
	got, err := resume(context.Background())
	if err != nil || len(got.Contenders) != 1 {
		t.Fatalf("resume = %+v, %v; want b named", got, err)
	}
	if !a.State().Acquired {
		t.Fatal("the call is not under the claim again after it was answered")
	}
	b.BeginRun()
	if err := within(t, b, []string{file}, 200*time.Millisecond); err == nil {
		t.Fatal("the resumed call no longer holds against other sessions")
	}
}

func TestACallMidWriteKeepsTheClaimWhileAnotherCallAsks(t *testing.T) {
	a, b, root := yieldPair(t)
	file := filepath.Join(root, "x.go")
	a.BeginRun()
	b.BeginRun()
	_, endWriter, err := a.HoldPaths(context.Background(), []string{file})
	if err != nil {
		t.Fatal(err)
	}
	defer endWriter()
	askCtx, endAsker, err := a.HoldPaths(context.Background(), []string{file})
	if err != nil {
		t.Fatal(err)
	}
	defer endAsker()
	resume := a.Yield(askCtx)
	if !a.State().Acquired {
		t.Fatal("the claim was given back under a call that is writing")
	}
	if err := within(t, b, []string{file}, 200*time.Millisecond); err == nil {
		t.Fatal("b wrote under a call that is mid-write")
	}
	if _, err := resume(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestABlockedAcquisitionFailsTypedAtTheMaximumWait(t *testing.T) {
	root, locks := t.TempDir(), t.TempDir()
	a, err := New(root, locks, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(root, locks, nil, WithMaxWait(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	named(a, "alpha", "sess-a")
	t.Cleanup(func() {
		a.EndRun()
		b.EndRun()
	})
	file := filepath.Join(root, "x.go")
	a.BeginRun()
	b.BeginRun()
	if err := a.AcquirePaths(context.Background(), []string{file}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = b.AcquirePaths(context.Background(), []string{file})
	var timeout *ClaimTimeoutError
	if !errors.As(err, &timeout) || !errors.Is(err, ErrClaimTimeout) {
		t.Fatalf("err = %v, want ClaimTimeoutError", err)
	}
	if timeout.RefusalCode() != CodeClaimTimeout || timeout.Conflict == nil || timeout.Conflict.Holder != "alpha" {
		t.Fatalf("timeout = %+v, want the holder named", timeout)
	}
	if waited := time.Since(start); waited < 250*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("gave up after %v, want about 300ms", waited)
	}
	if b.State().Waiting {
		t.Fatal("still marked waiting after giving up")
	}
}

func TestWithoutAMaximumWaitABlockedAcquisitionWaitsForTheCaller(t *testing.T) {
	a, b, root := yieldPair(t)
	file := filepath.Join(root, "x.go")
	a.BeginRun()
	b.BeginRun()
	if err := a.AcquirePaths(context.Background(), []string{file}); err != nil {
		t.Fatal(err)
	}
	err := within(t, b, []string{file}, 400*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrClaimTimeout) {
		t.Fatalf("err = %v, want the caller's deadline", err)
	}
}

func claimLog(a *Owner) string { return a.claimLogPath() }

func TestAClaimIsGrantedWhenTheLogCannotBeWrittenAndTheResumeSaysItCannotTell(t *testing.T) {
	a, b, root := yieldPair(t)
	named(a, "alpha", "sess-a")
	named(b, "beta", "sess-b")
	file := filepath.Join(root, "x.go")
	a.BeginRun()
	b.BeginRun()
	if err := a.AcquirePaths(context.Background(), []string{file}); err != nil {
		t.Fatal(err)
	}
	resume := a.Yield(context.Background())
	if err := os.RemoveAll(claimLog(a)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(claimLog(a), "block"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := b.AcquirePaths(context.Background(), []string{file}); err != nil {
		t.Fatalf("an unwritable claim log refused a claim: %v", err)
	}
	b.EndRun()
	got, err := resume(context.Background())
	if err != nil || !got.Unknown {
		t.Fatalf("resume = %+v, %v; want the record reported as unreadable, not as no contention", got, err)
	}
}

func TestACorruptClaimLogIsUnknownNotEmpty(t *testing.T) {
	a, b, root := yieldPair(t)
	named(a, "alpha", "sess-a")
	named(b, "beta", "sess-b")
	a.BeginRun()
	b.BeginRun()
	if err := a.AcquirePaths(context.Background(), []string{filepath.Join(root, "x.go")}); err != nil {
		t.Fatal(err)
	}
	resume := a.Yield(context.Background())
	if err := os.WriteFile(claimLog(a), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.AcquirePaths(context.Background(), []string{filepath.Join(root, "other.go")}); err != nil {
		t.Fatal(err)
	}
	b.EndRun()
	got, err := resume(context.Background())
	if err != nil || !got.Unknown {
		t.Fatalf("resume = %+v, %v; a log started over after corruption must read as unknown", got, err)
	}
}
