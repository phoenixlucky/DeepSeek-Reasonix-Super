package sessioninbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

// A queue another run owns must not be adopted through the lock-free fallback:
// the locked path takes a foreign queue over only after marking its in-flight
// items uncertain, so without the transaction the reader has to refuse.
func TestForeignRunQueueIsNotAdoptedWithoutTheTransaction(t *testing.T) {
	session := filepath.Join(testenv.TempDir(t), "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	owner, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	foreignRun := owner.runID + "-other"
	foreign := &Store{dir: owner.Dir(), limits: Limits{}.withDefaults(), runID: foreignRun, man: emptyManifest(foreignRun)}
	if _, err := foreign.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "queued by the other run"}}); err != nil {
		t.Fatal(err)
	}

	holdTransactionLock(t, owner)
	owner.mu.Lock()
	owner.man = emptyManifest(owner.runID)
	owner.mu.Unlock()

	meta, ok, readErr := owner.NextQueued()
	if ok {
		t.Fatalf("adopted a queue owned by another run: meta=%+v err=%v", meta, readErr)
	}
	if !errors.Is(readErr, ErrForeignRun) {
		t.Fatalf("the foreign-run refusal lost its identity: %v", readErr)
	}
}
