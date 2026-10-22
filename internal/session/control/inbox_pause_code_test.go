package control

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/sessioninbox"
)

func pauseTestController(t *testing.T) *Controller {
	t.Helper()
	dir := testenv.TempDir(t)
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{SessionPath: session, SessionDir: dir, WorkspaceRoot: dir})
	t.Cleanup(c.Close)
	return c
}

func TestPausingFromTheClientIsNamedAsTheUsers(t *testing.T) {
	c := pauseTestController(t)
	if err := c.SetInboxPaused(true); err != nil {
		t.Fatal(err)
	}
	if snap := c.InboxSnapshot(); !snap.Paused || snap.PauseCode != sessioninbox.PauseUser {
		t.Fatalf("snapshot = paused %v code %q, want paused by the user", snap.Paused, snap.PauseCode)
	}
	if err := c.SetInboxPaused(false); err != nil {
		t.Fatal(err)
	}
	if snap := c.InboxSnapshot(); snap.Paused || snap.PauseCode != "" {
		t.Fatalf("after resuming: paused %v code %q, want neither", snap.Paused, snap.PauseCode)
	}
}

func TestAQueuedLineWhoseReferenceCannotBeReadHoldsTheQueueAndSaysSo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not make a file unreadable on Windows")
	}
	c := pauseTestController(t)
	locked := filepath.Join(c.WorkspaceRoot(), "locked.md")
	if err := os.WriteFile(locked, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentFollowup, Submit: "look at @locked.md"}); err != nil {
		t.Fatal(err)
	}
	if snap := c.InboxSnapshot(); !snap.Paused || snap.PauseCode != sessioninbox.PauseReferenceBlocked {
		t.Fatalf("snapshot = paused %v code %q, want held by an unreadable reference", snap.Paused, snap.PauseCode)
	}
}

func TestLeavingASessionPausesItsQueueAsSessionChanged(t *testing.T) {
	c := pauseTestController(t)
	if _, err := c.EnqueueInbox(InboxRequest{Intent: sessioninbox.IntentFollowup, Submit: "waiting"}); err != nil {
		t.Fatal(err)
	}
	c.pauseInboxOnRotate()
	if snap := c.InboxSnapshot(); !snap.Paused || snap.PauseCode != sessioninbox.PauseSessionChanged {
		t.Fatalf("snapshot = paused %v code %q, want paused by the session changing", snap.Paused, snap.PauseCode)
	}
}
