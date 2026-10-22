package memory

import (
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// The aside copy is kept for the person, not for the store: its name does not end
// in .md, so after the repair the facts the store serves are exactly the fact
// files and the derived index never names the aside.
func TestAsideCopyIsNotServedAsAFact(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	if _, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "d-alpha", Body: "b"}, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	makeIndexUnreadable(t, store.Dir, "# Memory\n\nMy own note, not a fact.\n")
	if _, err := store.MigrateV2WithVersion("2"); err != nil {
		t.Fatalf("the boot pass did not repair the unreadable index: %v", err)
	}
	if aside := indexAsides(t, store.Dir); len(aside) != 1 {
		t.Fatalf("expected one aside copy, got %v", aside)
	}

	facts, err := store.List()
	if err != nil {
		t.Fatalf("the store cannot list its facts after the repair: %v", err)
	}
	if len(facts) != 1 || facts[0].Name != "alpha" {
		t.Fatalf("the aside copy leaked into the served facts: %+v", facts)
	}
	idx, err := store.Index()
	if err != nil {
		t.Fatalf("the index cannot be read after the repair: %v", err)
	}
	if strings.Contains(idx, "unreadable-") {
		t.Fatalf("the derived index names the aside copy: %q", idx)
	}
}
