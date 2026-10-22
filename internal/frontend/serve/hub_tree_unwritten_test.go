package serve

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/sessionstore"
)

// A path is minted at the first submit and written after it. A first turn in
// flight therefore has no transcript to list, and without a row the running
// conversation can be neither found in the sidebar nor returned to.
func TestTreeListsAHeldSessionBeforeItIsWritten(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	rt := hubRuntime(t, h, root)
	blank := hubRuntime(t, h, root)

	path := sessionstore.NewSessionPath(SessionDirFor(root), "test")
	rt.Server.Controller().SetSessionPath(path)

	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	tree := hubGet[[]treeWorkspace](t, srv, "/tree")
	if len(tree) != 1 {
		t.Fatalf("/tree = %+v, want the one workspace", tree)
	}
	var held []treeSession
	for _, s := range tree[0].Sessions {
		if s.RuntimeID == blank.ID {
			t.Fatalf("a pane that has minted nothing got a row: %+v", s)
		}
		if s.RuntimeID == rt.ID {
			held = append(held, s)
		}
	}
	if len(held) != 1 || filepath.Clean(held[0].Path) != filepath.Clean(path) {
		t.Fatalf("held rows = %+v, want the one unwritten session at %s", held, path)
	}
}

// A pane on a conversation that is on disk but outside the window is not
// "unwritten": the window leaving its row out is the view's choice, and adding
// a second row for it would list one conversation twice.
func TestTreeDoesNotAddAnOpenSessionTheWindowLeftOut(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	rt := hubRuntime(t, h, root)
	paths := seedAged(t, root, 120)
	oldest := paths[0]
	rt.Server.Controller().SetSessionPath(oldest)

	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	ws := treeFor(t, srv, "")
	var rows int
	for _, s := range ws.Sessions {
		if filepath.Clean(s.Path) == filepath.Clean(oldest) {
			rows++
		}
	}
	if rows > 1 {
		t.Fatalf("the open conversation was listed %d times", rows)
	}
}
