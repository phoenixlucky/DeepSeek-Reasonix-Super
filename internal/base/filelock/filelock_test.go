package filelock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

func registrySize() int {
	localRegistry.Lock()
	defer localRegistry.Unlock()
	return len(localRegistry.locks)
}

func TestAcquireHonorsDeadlineAndRecoversAfterRelease(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "state.lock")
	release, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Acquire(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contended acquire error = %v, want deadline exceeded", err)
	}

	release()
	secondRelease, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	secondRelease()
}

// The deadline bounds a wait. A caller whose budget was spent before it got
// here, by its own slow writes, still takes a lock that nobody holds.
func TestAcquireTakesAFreeLockAfterTheDeadline(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "state.lock")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 32 {
		release, err := Acquire(ctx, path)
		if err != nil {
			t.Fatalf("a free lock was refused because the deadline had passed: %v", err)
		}
		release()
	}
}

// A lock that is held still fails at once when the deadline has passed.
func TestAcquireRefusesAHeldLockAfterTheDeadline(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "state.lock")
	release, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Acquire(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("held lock after the deadline: %v, want context.Canceled", err)
	}
}

func TestExternalTimeoutBoundsOnlyFileLockRetries(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "state.lock")
	releaseExternal, err := tryLockFile(path)
	if err != nil {
		t.Fatalf("hold external file lock: %v", err)
	}
	defer releaseExternal()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	_, err = acquire(ctx, path, 60*time.Millisecond)
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("external acquire error = %v, want deadline exceeded", err)
	}
	if elapsed >= time.Second {
		t.Fatalf("external acquire waited %v, want the short external budget", elapsed)
	}
}

func TestLocalRegistryReclaimsReleasedEntries(t *testing.T) {
	before := registrySize()
	path := filepath.Join(testenv.TempDir(t), "ephemeral.lock")
	release, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if registrySize() <= before {
		t.Fatal("registry should grow while lock is held")
	}
	release()
	if got := registrySize(); got != before {
		t.Fatalf("registry size after release = %d, want %d (reclaimed)", got, before)
	}

	// Re-acquire still works after reclaim.
	release2, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	release2()
	if got := registrySize(); got != before {
		t.Fatalf("registry size after second cycle = %d, want %d", got, before)
	}
}
