package serve

import (
	"net/http/httptest"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/sessionstore"
)

func TestArchivedScopeListsArchivedRowsWhenActiveOnesAreNewer(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	const archived, active = 150, 27
	paths := seedAged(t, root, archived+active)
	for _, p := range paths[:archived] {
		if err := sessionstore.SetSessionLineageArchived(p, true); err != nil {
			t.Fatal(err)
		}
	}
	got := treeFor(t, srv, "?scope=archived&limit=50")
	if got.Total != archived+active || got.Archived != archived {
		t.Fatalf("counts: total %d archived %d", got.Total, got.Archived)
	}
	if len(got.Sessions) != 50 || got.Remaining != archived-50 {
		t.Fatalf("archived view: %d rows, remaining %d; want 50 rows and %d remaining", len(got.Sessions), got.Remaining, archived-50)
	}
	for _, row := range got.Sessions {
		if !row.Archived {
			t.Fatalf("an active row %s in the archived view", row.Name)
		}
	}
}
