package control

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
)

// TestDispatchKeepsQueuedWorkAliveWhileTheTransactionIsUnavailable pins the
// dispatcher half of the contract: while the disk transaction cannot run, a
// scan must schedule the next retry instead of reporting the inbox idle, and
// the durable item must survive until the transaction works again. Delivery
// happens on a retry inside the existing bounded burst; nothing past it.
func TestDispatchKeepsQueuedWorkAliveWhileTheTransactionIsUnavailable(t *testing.T) {
	dir := testenv.TempDir(t)
	inputs := make(chan string, 8)
	c := New(Options{
		Runner:      inboxReadFailureRunner{inputs: inputs},
		Sink:        event.FuncSink(func(event.Event) {}),
		SessionDir:  dir,
		SessionPath: filepath.Join(dir, "session.jsonl"),
	})
	t.Cleanup(func() {
		c.Close()
		c.autosaveWG.Wait()
	})

	const prompt = "must not be lost to a failed scan"
	if _, err := c.EnqueueInbox(InboxRequest{Submit: prompt}); err != nil {
		t.Fatal(err)
	}
	st, err := c.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	scans, scansFound := 0, 0
	retries := make(chan func(), 8)
	c.inbox.mu.Lock()
	c.inbox.afterDispatchScan = func(found bool) {
		mu.Lock()
		defer mu.Unlock()
		scans++
		if found {
			scansFound++
		}
	}
	// Take over the retry timer so the schedule is driven by this test instead
	// of by wall-clock delays.
	c.inbox.scheduleDispatchRetry = func(_ time.Duration, retry func()) {
		retries <- retry
	}
	c.inbox.mu.Unlock()

	lock := filepath.Join(st.Dir(), "transaction.lock")
	if err := os.Remove(lock); err != nil {
		t.Fatalf("remove lock file: %v", err)
	}
	if err := os.Mkdir(lock, 0o700); err != nil {
		t.Fatalf("replace lock with directory: %v", err)
	}

	returned := make(chan struct{})
	go func() {
		c.maybeDispatchInbox()
		close(returned)
	}()
	select {
	case got := <-inputs:
		t.Fatalf("dispatched %q through a transaction that cannot have run", got)
	case <-returned:
	}

	// Spend two of the bounded burst's three retries while the transaction is
	// still broken. Each failed scan must have scheduled the next one.
	for i := range 2 {
		select {
		case retry := <-retries:
			retry()
		case <-time.After(2 * time.Second):
			t.Fatalf("broken transaction scan %d scheduled no retry; it reported the inbox idle", i+1)
		}
	}
	mu.Lock()
	scanCount, foundCount := scans, scansFound
	mu.Unlock()
	if scanCount != 3 || foundCount != 3 {
		t.Fatalf("scans = %d (found %d), want 3 scans that all saw the queued item", scanCount, foundCount)
	}
	if snap := c.InboxSnapshot(); len(snap.Items) != 1 {
		t.Fatalf("queued item disappeared while the transaction was unavailable: %+v", snap)
	}
	select {
	case got := <-inputs:
		t.Fatalf("dispatched %q through a transaction that cannot have run", got)
	default:
	}

	// Repair the transaction path while the burst still has its third retry
	// pending: that already-scheduled retry must deliver the item with no new
	// user-visible kick.
	if err := os.Remove(lock); err != nil {
		t.Fatalf("remove lock directory: %v", err)
	}
	select {
	case retry := <-retries:
		retry()
	case <-time.After(2 * time.Second):
		t.Fatal("the broken scans stopped scheduling retries inside the bounded burst")
	}
	select {
	case got := <-inputs:
		if got != prompt {
			t.Fatalf("dispatched input = %q", got)
		}
	case <-time.After(3 * time.Second):
		mu.Lock()
		finalScans := scans
		mu.Unlock()
		t.Fatalf("queued prompt was never dispatched after the transaction was repaired: "+
			"dispatcher stopped after %d scans while the durable item stayed queued", finalScans)
	}
}

type inboxReadFailureRunner struct {
	inputs chan string
}

func (r inboxReadFailureRunner) Run(_ context.Context, input string) error {
	r.inputs <- input
	return nil
}
