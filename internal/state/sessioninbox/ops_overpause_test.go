package sessioninbox

import (
	"errors"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestClaimItemOverPauseTakesOneItemAndKeepsThePause(t *testing.T) {
	s, err := Open(filepath.Join(testenv.TempDir(t), "s.jsonl"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	var ids []string
	for _, text := range []string{"older", "newer"} {
		rec, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: text}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ItemID)
	}
	if err := s.Pause(PauseUser); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimItem(ids[0]); !errors.Is(err, ErrPaused) {
		t.Fatalf("ClaimItem on a paused queue = %v, want ErrPaused", err)
	}
	if err := s.ClaimItemOverPause(ids[1]); err != nil {
		t.Fatalf("ClaimItemOverPause: %v", err)
	}
	snap := s.Snapshot()
	if !snap.Paused {
		t.Fatal("the pause was lifted")
	}
	for _, it := range snap.Items {
		want := StateQueued
		if it.ID == ids[1] {
			want = StateRunning
		}
		if it.State != want {
			t.Fatalf("item %s is %s, want %s", it.ID, it.State, want)
		}
	}
}

func TestOnlyPausesTheHostSetToGuardStaleInputCanBeOverriddenByAStandingConsent(t *testing.T) {
	for code, want := range map[PauseCode]bool{
		PauseSessionChanged: true, PauseReferenceBlocked: true, PauseRecovered: true,
		PauseUser: false, PauseWriteAuthority: false, PauseTurnRecord: false, "": false, "from_a_newer_kernel": false,
	} {
		if got := code.GuardsStaleInput(); got != want {
			t.Errorf("%q.GuardsStaleInput() = %v, want %v", code, got, want)
		}
	}
}
