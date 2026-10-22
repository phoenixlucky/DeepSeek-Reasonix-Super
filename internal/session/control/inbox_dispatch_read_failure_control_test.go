package control

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
)

// TestDispatchRetriesWhenTheQueueCannotBeRead pins the dispatcher half of the
// contract: a durable queue that exists but cannot be read must schedule a
// retry, not report the inbox idle. The manifest is a directory here, so both
// the transaction and the lock-free snapshot read fail while the lock itself
// stays acquirable.
func TestDispatchRetriesWhenTheQueueCannotBeRead(t *testing.T) {
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

	if _, err := c.EnqueueInbox(InboxRequest{Submit: "queued but unreadable"}); err != nil {
		t.Fatal(err)
	}
	st, err := c.ensureInbox()
	if err != nil {
		t.Fatal(err)
	}
	retries := make(chan func(), 4)
	scans := 0
	c.inbox.mu.Lock()
	c.inbox.afterDispatchScan = func(bool) { scans++ }
	c.inbox.scheduleDispatchRetry = func(_ time.Duration, retry func()) { retries <- retry }
	c.inbox.mu.Unlock()

	manifest := filepath.Join(st.Dir(), "manifest.json")
	if err := os.Remove(manifest); err != nil {
		t.Fatalf("remove manifest: %v", err)
	}
	if err := os.Mkdir(manifest, 0o700); err != nil {
		t.Fatalf("replace manifest with directory: %v", err)
	}

	c.maybeDispatchInbox()

	select {
	case got := <-inputs:
		t.Fatalf("dispatched %q through a queue that cannot be read", got)
	default:
	}
	c.inbox.mu.Lock()
	scanCount := scans
	c.inbox.mu.Unlock()
	if scanCount != 1 {
		t.Fatalf("dispatch scans = %d, want 1 (the read must reach NextQueued)", scanCount)
	}
	select {
	case <-retries:
	default:
		t.Fatal("an unreadable queue ended dispatch with no retry; it was reported idle")
	}
}
