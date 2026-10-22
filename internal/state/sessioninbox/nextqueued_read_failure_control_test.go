package sessioninbox

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

// breakTransactionLock makes every transaction fail without waiting for the
// 5s lock timeout: the lock path becomes a directory, so Acquire's open fails
// with ENOTDIR/EISDIR instead of contending. The published manifest is left
// untouched and readable.
func breakTransactionLock(t *testing.T, s *Store) {
	t.Helper()
	path := filepath.Join(s.Dir(), diskLockName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove lock file: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("replace lock with directory: %v", err)
	}
}

func openControlStore(t *testing.T, text string) (*Store, string) {
	t.Helper()
	session := filepath.Join(testenv.TempDir(t), "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if text != "" {
		if _, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: text}}); err != nil {
			t.Fatal(err)
		}
	}
	return s, session
}

// TestNextQueuedEmptyQueueStaysIdle pins the permanent condition: a queue that
// genuinely holds nothing is idle and not an error, even when the transaction
// path is broken, so a read failure cannot manufacture work.
func TestNextQueuedEmptyQueueStaysIdle(t *testing.T) {
	s, _ := openControlStore(t, "")
	breakTransactionLock(t, s)

	meta, ok, err := s.NextQueued()
	t.Logf("empty queue under a broken transaction: meta=%+v ok=%v err=%v", meta, ok, err)
	if err != nil {
		t.Fatalf("empty queue reported a read error: %v", err)
	}
	if ok {
		t.Fatalf("empty queue reported work: %+v", meta)
	}
}

// TestNextQueuedPausedQueueStaysIdle pins the other permanent condition: the
// lock-free snapshot read still honours the pause flag, so a queue parked for
// inspection is idle rather than dispatched.
func TestNextQueuedPausedQueueStaysIdle(t *testing.T) {
	s, _ := openControlStore(t, "held for inspection")
	if err := s.Pause(PauseUser); err != nil {
		t.Fatal(err)
	}
	breakTransactionLock(t, s)

	meta, ok, err := s.NextQueued()
	t.Logf("paused queue under a broken transaction: meta=%+v ok=%v err=%v", meta, ok, err)
	if err != nil {
		t.Fatalf("paused queue reported a read error: %v", err)
	}
	if ok {
		t.Fatalf("paused queue reported dispatchable work: %+v", meta)
	}
}

// TestNextQueuedUnreadableSnapshotSurfacesError pins the error side of the
// contract: when the transaction cannot run and the published manifest cannot
// be read either, the caller is told the read failed instead of being told the
// queue is empty.
func TestNextQueuedUnreadableSnapshotSurfacesError(t *testing.T) {
	s, _ := openControlStore(t, "still queued")
	manifest := filepath.Join(s.Dir(), manifestName)
	if err := os.Remove(manifest); err != nil {
		t.Fatalf("remove manifest: %v", err)
	}
	if err := os.Mkdir(manifest, 0o700); err != nil {
		t.Fatalf("replace manifest with directory: %v", err)
	}

	meta, ok, err := s.NextQueued()
	t.Logf("unreadable queue: meta=%+v ok=%v err=%v", meta, ok, err)
	if err == nil {
		t.Fatalf("unreadable queue was reported as readable: ok=%v meta=%+v", ok, meta)
	}
	if ok {
		t.Fatalf("unreadable queue reported work: %+v", meta)
	}
}
