package serve

import (
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/sessionstore"
)

func TestOrphanPanesNamesAPaneWhoseConversationHasNoRow(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	listed := hubRuntime(t, h, root)
	lost := hubRuntime(t, h, root)
	blank := hubRuntime(t, h, root)
	for _, rt := range []*Runtime{listed, lost} {
		rt.Server.Controller().SetSessionPath(sessionstore.NewSessionPath(SessionDirFor(root), "test"))
	}
	tree := []treeWorkspace{{Root: root, Sessions: []treeSession{{RuntimeID: listed.ID}}}}

	got := h.orphanPanes(tree)
	if len(got) != 1 || got[0].Runtime != lost.ID {
		t.Fatalf("orphanPanes = %+v, want only %s (a pane that has minted no path, %s, has nothing to list)", got, lost.ID, blank.ID)
	}
	if got[0].Workspace == "" || got[0].Session == "" {
		t.Fatalf("the report must carry the pane's folder and its conversation path: %+v", got[0])
	}
}
