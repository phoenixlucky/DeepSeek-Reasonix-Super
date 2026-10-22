package control

import (
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/peermail"
)

func threadOf(t *testing.T, c *Controller, key string) PeerThread {
	t.Helper()
	for _, th := range c.PeerThreads() {
		if th.Key == key {
			return th
		}
	}
	t.Fatalf("no thread with %s among %+v", key, c.PeerThreads())
	return PeerThread{}
}

// The whole life of one hand-off, seen from the sender: waiting while the other
// user has not decided, working once they took it, replied when they write back.
func TestAThreadMovesFromWaitingToWorkingToReplied(t *testing.T) {
	peersDir := filepath.Join(testenv.TempDir(t), "peers")
	a := newPeerSession(t, peersDir, "alpha")
	b := newPeerSession(t, peersDir, "beta")
	a.peerIngest()
	if got := a.PeerThreads(); len(got) != 0 {
		t.Fatalf("threads before sending anything: %+v", got)
	}

	sent, err := a.SendPeerMessage(b.peers.key, "look at the build")
	if err != nil {
		t.Fatal(err)
	}
	if th := threadOf(t, a, b.peers.key); th.State != PeerWaiting || th.MessageID != sent.ID || th.Title == "" {
		t.Fatalf("after sending: %+v, want waiting on %s with the peer's name", th, sent.ID)
	}

	b.peerIngest()
	if err := b.ResolvePeerMessage(sent.ID, true); err != nil {
		t.Fatal(err)
	}
	if th := threadOf(t, a, b.peers.key); th.State != PeerWorking {
		t.Fatalf("after the other user took it: %+v, want working", th)
	}

	time.Sleep(10 * time.Millisecond)
	if _, err := b.SendPeerMessage(a.peers.key, "build is fixed"); err != nil {
		t.Fatal(err)
	}
	a.peerIngest()
	if th := threadOf(t, a, b.peers.key); th.State != PeerReplied {
		t.Fatalf("after the reply arrived: %+v, want replied", th)
	}

	time.Sleep(10 * time.Millisecond)
	if _, err := a.SendPeerMessage(b.peers.key, "thanks, one more thing"); err != nil {
		t.Fatal(err)
	}
	if th := threadOf(t, a, b.peers.key); th.State != PeerWaiting {
		t.Fatalf("after sending again: %+v, want waiting on the new message", th)
	}
}

func TestADeniedMessageIsDeclinedAndReleasedCountsAsTaken(t *testing.T) {
	now := time.Now().UTC()
	at := func(s int) time.Time { return now.Add(time.Duration(s) * time.Second) }
	trail := []peermail.AuditRecord{
		{At: at(1), Receiver: "s-b", Sender: "s-a", ID: "m1", Decision: peermail.DecisionSent},
		{At: at(2), Receiver: "s-b", Sender: "s-a", ID: "m1", Decision: peermail.DecisionDenied},
		{At: at(3), Receiver: "s-c", Sender: "s-a", ID: "m2", Decision: peermail.DecisionSent},
		{At: at(4), Receiver: "s-c", Sender: "s-a", ID: "m2", Decision: peermail.DecisionReleased},
		{At: at(5), Receiver: "s-d", Sender: "s-x", ID: "m3", Decision: peermail.DecisionSent}, // someone else's
	}
	got := map[string]PeerThreadState{}
	for _, th := range peerThreadsFrom("s-a", trail) {
		got[th.Key] = th.State
	}
	if len(got) != 2 || got["s-b"] != PeerDeclined || got["s-c"] != PeerWorking {
		t.Fatalf("threads = %v, want s-b declined, s-c working, and nothing for a stranger's message", got)
	}
}

func TestOnlyTheLatestMessageDecidesAndNewestComesFirst(t *testing.T) {
	now := time.Now().UTC()
	at := func(s int) time.Time { return now.Add(time.Duration(s) * time.Second) }
	trail := []peermail.AuditRecord{
		{At: at(1), Receiver: "s-b", Sender: "s-a", ID: "old", Decision: peermail.DecisionSent},
		{At: at(2), Receiver: "s-b", Sender: "s-a", ID: "old", Decision: peermail.DecisionDenied},
		{At: at(3), Receiver: "s-b", Sender: "s-a", ID: "new", Decision: peermail.DecisionSent},
		{At: at(6), Receiver: "s-c", Sender: "s-a", ID: "c1", Decision: peermail.DecisionSent},
	}
	got := peerThreadsFrom("s-a", trail)
	if len(got) != 2 || got[0].Key != "s-c" || got[1].Key != "s-b" || got[1].State != PeerWaiting || got[1].MessageID != "new" {
		t.Fatalf("threads = %+v, want s-c first and s-b waiting on its new message", got)
	}
}
