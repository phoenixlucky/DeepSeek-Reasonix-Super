package sessioninbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/store"
)

func TestAPauseKeepsItsFirstHostCauseThePersonsPauseTakesOverAndResumeClearsIt(t *testing.T) {
	session := filepath.Join(testenv.TempDir(t), "s.jsonl")
	s, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(PauseReferenceBlocked); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(PauseSessionChanged); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().PauseCode; got != PauseReferenceBlocked {
		t.Fatalf("code = %q, want the first host cause kept", got)
	}
	if err := s.Pause(PauseUser); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(PauseSessionChanged); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().PauseCode; got != PauseUser {
		t.Fatalf("code = %q, want the person's pause to take the cause over", got)
	}
	s.Close()

	s, err = Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if snap := s.Snapshot(); !snap.Paused || snap.PauseCode != PauseUser {
		t.Fatalf("reopened: paused %v code %q, want the cause persisted", snap.Paused, snap.PauseCode)
	}
	if err := s.Resume(); err != nil {
		t.Fatal(err)
	}
	if snap := s.Snapshot(); snap.Paused || snap.PauseCode != "" {
		t.Fatalf("resumed: paused %v code %q, want neither", snap.Paused, snap.PauseCode)
	}
}

func TestAQueueFoundWithLeftoversOnOpenIsPausedAsRecovered(t *testing.T) {
	session := filepath.Join(testenv.TempDir(t), "s.jsonl")
	s, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enqueue(EnqueueRequest{Envelope: PromptEnvelope{SubmitText: "left behind"}}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	manPath := filepath.Join(store.SessionInboxDir(session), "manifest.json")
	data, err := os.ReadFile(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manPath, []byte(strings.Replace(string(data), ProcessRunID(), "other-run-id-0000", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if snap := s.Snapshot(); !snap.Paused || snap.PauseCode != PauseRecovered {
		t.Fatalf("reopened: paused %v code %q, want paused as recovered", snap.Paused, snap.PauseCode)
	}
}
