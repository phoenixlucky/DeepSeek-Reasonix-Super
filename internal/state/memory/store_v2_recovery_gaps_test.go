package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// A staged copy pairs with the retired copy of the SAME publish call, identified
// by its stamp. A staged copy carrying the id but no retired twin from that call
// is not evidence of an interrupted rename: pairing it would restore bytes from a
// different publish, so the store must leave everything untouched.
func TestInterruptedRenameDoesNotPairACopyFromAnotherPublish(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	intent := Memory{ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2"}
	opts := SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}
	if killed := killDuring(t, isIndexPublish, func() { _, _ = store.SaveWithOptions(intent, opts) }); !killed {
		t.Fatal("the probe never reached the index publication, so the window was not exercised")
	}
	staged, _ := filepath.Glob(filepath.Join(store.Dir, ".staged-*"))
	if len(staged) != 1 {
		t.Fatalf("expected one staged copy, got %v", staged)
	}
	restamped := filepath.Join(store.Dir, strings.Replace(filepath.Base(staged[0]), "-", "-999", 1)+"-foreign")
	if err := os.Rename(staged[0], restamped); err != nil {
		t.Fatal(err)
	}

	if _, _, ok, err := store.recoverInterruptedRename(first.Memory.ID); ok || err != nil {
		t.Fatalf("paired a staged copy with a retired copy from another publish: ok=%v err=%v", ok, err)
	}
	for _, name := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(store.Dir, name)); !os.IsNotExist(err) {
			t.Fatalf("the store wrote %s from an unpaired copy: %v", name, err)
		}
	}
	if _, err := os.Stat(restamped); err != nil {
		t.Fatalf("the unpaired staged copy was moved or removed: %v", err)
	}
}

// A fact created at the old name after the interrupted rename owns that name. The
// retry must refuse instead of overwriting it, and must leave the interrupted
// artifacts in place so a later retry (once the name is free) can still recover.
func TestInterruptedRenameRefusesWhenTheOldNameIsNowTaken(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	intent := Memory{ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2"}
	opts := SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}
	if killed := killDuring(t, isIndexPublish, func() { _, _ = store.SaveWithOptions(intent, opts) }); !killed {
		t.Fatal("the probe never reached the index publication, so the window was not exercised")
	}
	later, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "later", Body: "later"}, SaveOptions{})
	if err != nil {
		t.Fatalf("a later fact could not take the freed old name: %v", err)
	}

	if _, retryErr := store.SaveWithOptions(intent, opts); retryErr == nil {
		t.Fatalf("the retry overwrote a fact that now holds the old name")
	} else if !strings.Contains(retryErr.Error(), "now active") {
		t.Fatalf("the refusal does not name the conflict: %v", retryErr)
	}
	raw, err := os.ReadFile(filepath.Join(store.Dir, "alpha.md"))
	if err != nil || !strings.Contains(string(raw), "later") {
		t.Fatalf("the later fact's bytes were overwritten: %q err=%v", raw, err)
	}
	if got, ok, err := store.Read(later.Memory.ID); !ok || err != nil || got.Body != "later" {
		t.Fatalf("the later fact is no longer readable by its own id: ok=%v body=%q", ok, got.Body)
	}
	if staged, _ := filepath.Glob(filepath.Join(store.Dir, ".staged-*")); len(staged) != 1 {
		t.Fatalf("the interrupted copy was dropped while the name was taken: %v", staged)
	}
	if retired, _ := filepath.Glob(filepath.Join(store.Dir, ".retired-*")); len(retired) != 1 {
		t.Fatalf("the retired copy was dropped while the name was taken: %v", retired)
	}
}
