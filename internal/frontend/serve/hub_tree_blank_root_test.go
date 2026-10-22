package serve

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

// A launcher whose working directory is the home folder opens a blank pane
// there. That folder was never picked and holds nothing, so listing it makes
// the sidebar show a project the user cannot get rid of: removing it only
// forgets a folder that was never remembered.
func TestTreeOmitsAnUnpickedLaunchFolderThatHoldsNothing(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	home := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, home)
	if err := os.MkdirAll(SessionDirFor(home), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(SessionDirFor(home), "blank.jsonl"), []byte("{\"role\":\"system\",\"content\":\"sys\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	if tree := hubGet[[]treeWorkspace](t, srv, "/tree"); len(tree) != 0 {
		t.Fatalf("/tree = %+v, want no folder: nothing was picked and nothing was said", tree)
	}
}

func TestTreeKeepsARememberedFolderThatHoldsNothing(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	if err := addRememberedWorkspace(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)

	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	tree := hubGet[[]treeWorkspace](t, srv, "/tree")
	if len(tree) != 1 || !tree[0].Remembered {
		t.Fatalf("/tree = %+v, want the one remembered folder", tree)
	}
}
