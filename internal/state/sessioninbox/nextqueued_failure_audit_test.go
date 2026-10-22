package sessioninbox

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/filelock"
	"reasonix/internal/base/testenv"
)

// holdTransactionLock takes the disk transaction lock the way a second Reasonix
// process does. While it is held, every Store transaction in this process fails
// the way it does in production on lock contention, on a lock file that cannot
// be created (permissions, ENOSPC), or on a transient filesystem error.
func holdTransactionLock(t *testing.T, s *Store) {
	t.Helper()
	release, err := filelock.TryAcquire(filepath.Join(s.Dir(), diskLockName))
	if err != nil {
		t.Fatalf("hold transaction lock: %v", err)
	}
	t.Cleanup(release)
}

// TestNextQueuedReportsLockContentionInsteadOfEmpty pins the contract that a
// queue read which could not run must not be reported as "nothing queued".
// The in-memory manifest is empty on purpose, so the durable queue on disk
// behind the contended lock is the only authority: NextQueued discarding the
// transaction error would report that work as absent and never retry it.
func TestNextQueuedReportsLockContentionInsteadOfEmpty(t *testing.T) {
	session := filepath.Join(testenv.TempDir(t), "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rec, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "queued prompt"}})
	if err != nil {
		t.Fatal(err)
	}

	// A second store advanced the durable queue: this store has not seen its
	// item. It carries this run's ID because an empty runID takes the
	// cross-process recovery branch, which pauses the queue (idle by contract).
	second := &Store{dir: s.Dir(), limits: Limits{}.withDefaults(), runID: s.runID, man: emptyManifest(s.runID)}
	if _, err := second.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "queued by the other process"}}); err != nil {
		t.Fatal(err)
	}

	holdTransactionLock(t, s)

	// Empty the in-memory snapshot, so NextQueued has no choice but to consult
	// the durable queue behind the transaction it cannot acquire: the state of a
	// store that has not yet observed the other writer.
	s.mu.Lock()
	s.man = emptyManifest(s.runID)
	s.mu.Unlock()

	// Every error-returning method does surface the failed transaction.
	if _, _, readErr := s.ReadItem(rec.ItemID); readErr == nil {
		t.Fatalf("lock contention did not break the transaction path")
	} else {
		t.Logf("transaction error on ReadItem: %v", readErr)
	}

	meta, ok, readErr := s.NextQueued()
	t.Logf("NextQueued under lock contention: meta=%+v ok=%v err=%v", meta, ok, readErr)
	if !ok {
		t.Fatalf("NextQueued reported an empty queue while the disk queue holds work and "+
			"the transaction failed: a read failure is being reported as absent work (err=%v)", readErr)
	}
	if meta.ID != rec.ItemID {
		t.Fatalf("NextQueued returned %q, want %q", meta.ID, rec.ItemID)
	}
}
