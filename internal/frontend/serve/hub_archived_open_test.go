package serve

import (
	"context"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

// Opening an archived conversation shows that conversation: it adds no second
// row to the list, and what the person archived stays archived until they say
// otherwise.
func TestOpeningAnArchivedSessionAddsNoRowAndKeepsItArchived(t *testing.T) {
	root := testenv.TempDir(t)
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	h := NewHub(HubOptions{ProviderResolver: homeResolver()})
	defer h.Shutdown()
	path := filepath.Join(SessionDirFor(root), "20261001-120000.jsonl")
	s := sessionstore.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "first topic of the archived talk"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "an answer"})
	if err := s.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	if err := sessionstore.SetSessionArchived(path, true); err != nil {
		t.Fatal(err)
	}

	rt, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: path})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { rt.Server.Controller().Close() })

	if got := rt.Server.Controller().SessionPath(); mustEval(t, got) != mustEval(t, path) {
		t.Fatalf("the pane is on %q, want the archived session %q", got, path)
	}
	rows := h.workspacePage(root, "", nil, treeWindow{scope: scopeEvery}, 0).Sessions
	if len(rows) != 1 {
		t.Fatalf("the list has %d rows after opening one archived session: %+v", len(rows), rows)
	}
	if !rows[0].Archived || rows[0].Path != path {
		t.Fatalf("the opened row is %+v, want the archived session itself", rows[0])
	}
	open := h.openSessionsIn(root)
	active := h.workspacePage(root, "", open, treeWindow{scope: scopeActive}, 0).Sessions
	if len(active) != 1 || active[0].Path != path || active[0].RuntimeID == "" || active[0].Title == "" || active[0].Turns == 0 {
		t.Fatalf("the everyday list shows %+v, want the open session once, with its own title and turns", active)
	}
}

// A pane's conversation keeps its row whatever the window's scope, word or size
// leaves out, so its tab can read its title from the row.
func TestAnOpenSessionOutsideTheWindowKeepsItsTitledRow(t *testing.T) {
	root := testenv.TempDir(t)
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	h := NewHub(HubOptions{ProviderResolver: homeResolver()})
	defer h.Shutdown()
	old := filepath.Join(SessionDirFor(root), "20260101-120000.jsonl")
	s := sessionstore.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "the oldest conversation"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "an answer"})
	if err := s.SaveSnapshot(old); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"20260301-120000.jsonl", "20260302-120000.jsonl"} {
		n := sessionstore.NewSession("system")
		n.Add(provider.Message{Role: provider.RoleUser, Content: "a newer one " + name})
		n.Add(provider.Message{Role: provider.RoleAssistant, Content: "ok"})
		if err := n.SaveSnapshot(filepath.Join(SessionDirFor(root), name)); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: old})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { rt.Server.Controller().Close() })

	rows := h.workspacePage(root, "", h.openSessionsIn(root), treeWindow{scope: scopeEvery}, 1).Sessions
	var held *treeSession
	for i := range rows {
		if rows[i].RuntimeID != "" {
			held = &rows[i]
		}
	}
	if held == nil || held.Title == "" || held.Turns == 0 {
		t.Fatalf("rows %+v: the open session has no titled row of its own", rows)
	}
	if len(rows) != 2 {
		t.Fatalf("rows %+v, want the one the window allows plus the open one", rows)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
