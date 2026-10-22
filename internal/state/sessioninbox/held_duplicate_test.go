package sessioninbox

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func heldStore(t *testing.T) *Store {
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
	return s
}

func send(t *testing.T, s *Store, text string, origin PromptOrigin, intent InboxIntent) InboxReceipt {
	t.Helper()
	rec, err := s.Enqueue(EnqueueRequest{Intent: intent, Origin: origin, Source: "test", Envelope: PromptEnvelope{DisplayText: text, SubmitText: text}})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// The same words sent again while the first copy is still waiting are the same
// instruction: a second copy would run as a second turn and be answered twice.
func TestAnIdenticalResendWhileTheFirstCopyWaitsIsNotAnotherItem(t *testing.T) {
	s := heldStore(t)
	first := send(t, s, "stop and write the handoff", OriginUser, IntentFollowup)
	second := send(t, s, "stop and write the handoff", OriginUser, IntentSteer)
	third := send(t, s, "stop and write the handoff", OriginUser, IntentFollowup)
	if !second.Idempotent || !third.Idempotent || second.ItemID != first.ItemID || third.ItemID != first.ItemID {
		t.Fatalf("receipts = %+v %+v, want both to point at the first item", second, third)
	}
	if n := len(s.Snapshot().Items); n != 1 {
		t.Fatalf("%d items waiting, want 1", n)
	}
	send(t, s, "something else", OriginUser, IntentFollowup)
	if n := len(s.Snapshot().Items); n != 2 {
		t.Fatalf("a different message must still queue: %d items", n)
	}
}

// A steer the stop left unapplied is the first copy: sending its words again
// is the retry, and it comes back as one waiting follow-up.
func TestResendingTheWordsOfAnUnappliedSteerRetriesIt(t *testing.T) {
	s := heldStore(t)
	first := send(t, s, "stop and write the handoff", OriginUser, IntentSteer)
	if err := s.SetStateCoded(first.ItemID, StateUncertain, BlockSteerUnapplied, "unapplied"); err != nil {
		t.Fatal(err)
	}
	again := send(t, s, "stop and write the handoff", OriginUser, IntentSteer)
	if !again.Idempotent || again.ItemID != first.ItemID {
		t.Fatalf("receipt = %+v, want the first item", again)
	}
	items := s.Snapshot().Items
	if len(items) != 1 || items[0].State != StateQueued || items[0].Intent != IntentFollowup || items[0].BlockCode != "" {
		t.Fatalf("items = %+v, want one queued follow-up with the block cleared", items)
	}
}

// The runtime's own notes are told apart by their idempotency key, not by their
// words, and a person's words are never merged into one.
func TestOnlyAPersonsOwnIdenticalWordsAreMerged(t *testing.T) {
	s := heldStore(t)
	send(t, s, "same words", OriginHost, IntentSteer)
	send(t, s, "same words", OriginHost, IntentSteer)
	send(t, s, "same words", OriginUser, IntentFollowup)
	if n := len(s.Snapshot().Items); n != 3 {
		t.Fatalf("%d items, want the two host notes and the person's one kept apart", n)
	}
}
