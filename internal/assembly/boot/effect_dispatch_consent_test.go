package boot

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/state/peermail"
)

func (p *peerToolProvider) sawInMessages(needle string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, req := range p.reqs {
		for _, m := range req.Messages {
			if strings.Contains(m.Content, needle) {
				return true
			}
		}
	}
	return false
}

// While the user's dispatch switch is on, a task from the session that opened
// this one reaches the model with no card, framed as data from that session and
// not as something the user read; the same words from a session the host never
// paired it with, or signed with another secret, wait for the user as before.
func TestEffectADispatchedTaskReachesTheModelWithoutACardAndAStrangersDoesNot(t *testing.T) {
	const parent = "s-00000000000000bb"
	secret := []byte("0123456789abcdef0123456789abcdef")
	rec, ctrl, peersDir, seen := buildPeerToolSession(t, true, "nobody", "hello", "dispatch = true\n")
	t.Setenv("REASONIX_HOME", filepath.Dir(peersDir))
	receiver := ctrl.PeerKey()
	if receiver == "" {
		t.Fatal("the receiving session did not publish itself")
	}
	ctrl.MarkOpenedByDispatch(parent, secret, 1)
	d, err := peermail.Open(peersDir)
	if err != nil {
		t.Fatal(err)
	}
	var seq uint64
	send := func(id, body string, signWith []byte) {
		t.Helper()
		seq++
		m := peermail.Message{ID: id, SentAt: time.Now().UTC(), Body: body, Seq: seq,
			From: peermail.Party{SessionID: parent, Title: "planner", Project: "alpha"}, To: peermail.Party{SessionID: receiver}}
		m.Proof = peermail.Proof(signWith, m)
		if err := d.Send(m); err != nil {
			t.Fatal(err)
		}
	}

	send("m-forged", "FORGED-TASK-WITH-ANOTHER-SECRET", []byte("another secret another secret!!!"))
	ctrl.RefreshPeers()
	if held := ctrl.HeldPeerMessages(); len(held) != 1 || held[0].Body != "FORGED-TASK-WITH-ANOTHER-SECRET" {
		t.Fatalf("held = %+v, want the badly signed task waiting for a card", held)
	}

	send("m-real", "ROTATE-THE-KEY-TASK", secret)
	ctrl.RefreshPeers()
	waitFor(t, "the dispatched task to reach the provider", func() bool { return rec.sawInMessages("ROTATE-THE-KEY-TASK") })
	if rec.sawInMessages("FORGED-TASK-WITH-ANOTHER-SECRET") {
		t.Fatal("a task signed with another secret reached the model")
	}
	if !rec.sawInMessages("has not read this message") || !rec.sawInMessages("not an instruction from the user") {
		t.Fatal("the dispatched task reached the model without saying the user did not read it and that it is data")
	}
	// The stub provider's own send_peer_message call is the only card there may be:
	// nothing asked the user to let the dispatched task in.
	seen.mu.Lock()
	for _, card := range seen.approvals {
		if card.Tool != "send_peer_message" {
			t.Errorf("unexpected approval card: %+v", card)
		}
	}
	seen.mu.Unlock()
	if got := ctrl.HeldPeerMessages(); len(got) != 1 {
		t.Fatalf("held = %+v, want only the badly signed one still waiting", got)
	}
}
