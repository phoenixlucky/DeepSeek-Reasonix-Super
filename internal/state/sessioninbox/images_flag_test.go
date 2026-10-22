package sessioninbox

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestSnapshotSaysWhichQueuedItemsCarryAnImage(t *testing.T) {
	dir := testenv.TempDir(t)
	session := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(session, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(session, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	add := func(text string, images []string) string {
		t.Helper()
		rec, err := s.Enqueue(EnqueueRequest{Intent: IntentSteer, Source: "test",
			Envelope: PromptEnvelope{DisplayText: text, SubmitText: text, FrozenImages: images}})
		if err != nil {
			t.Fatal(err)
		}
		return rec.ItemID
	}
	plain := add("only words", nil)
	pictured := add("look at this", []string{"data:image/png;base64,AAAA"})
	flag := func(id string) bool {
		for _, it := range s.Snapshot().Items {
			if it.ID == id {
				return it.Images
			}
		}
		t.Fatalf("item %s not in the snapshot", id)
		return false
	}
	if flag(plain) || !flag(pictured) {
		t.Fatalf("images flag: plain=%v pictured=%v, want false and true", flag(plain), flag(pictured))
	}
	if _, err := s.UpdateItem(pictured, PromptEnvelope{DisplayText: "words now", SubmitText: "words now"}); err != nil {
		t.Fatal(err)
	}
	if flag(pictured) {
		t.Fatal("an edit that dropped the picture still reads as carrying one")
	}
	if _, err := s.UpdateItem(plain, PromptEnvelope{DisplayText: "now with one", SubmitText: "now with one", FrozenImages: []string{"data:image/png;base64,BBBB"}}); err != nil {
		t.Fatal(err)
	}
	if !flag(plain) {
		t.Fatal("an edit that added a picture does not read as carrying one")
	}
}
