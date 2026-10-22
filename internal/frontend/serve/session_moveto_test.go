package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/sessionstore"
)

func moveTo(t *testing.T, srv *httptest.Server, path, target string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/tree/sessions/move-to", "application/json", strings.NewReader(fmt.Sprintf(`{"path":%q,"target":%q}`, path, target)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func twoProjects(t *testing.T) (*Hub, *httptest.Server, string, string) {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	a, b := testenv.TempDir(t), testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, a)
	hubRuntime(t, h, b)
	srv := httptest.NewServer(operatorHandler(h))
	t.Cleanup(srv.Close)
	return h, srv, a, b
}

func namesIn(t *testing.T, srv *httptest.Server, root string) []string {
	t.Helper()
	for _, ws := range hubGet[[]treeWorkspace](t, srv, "/tree") {
		if ws.Root == root {
			names := make([]string, len(ws.Sessions))
			for i, row := range ws.Sessions {
				names[i] = row.Name
			}
			return names
		}
	}
	return nil
}

func TestMovingAConversationToAnotherProjectMovesItInTheTree(t *testing.T) {
	_, srv, a, b := twoProjects(t)
	paths := seedAged(t, a, 2)

	status, out := moveTo(t, srv, paths[0], b)
	if status != http.StatusOK || out["originalRemoved"] != true {
		t.Fatalf("move = %d %v", status, out)
	}
	moved, _ := out["path"].(string)
	if filepath.Dir(moved) != SessionDirFor(b) {
		t.Fatalf("it landed in %s, want project b's folder %s", moved, SessionDirFor(b))
	}
	if got := namesIn(t, srv, a); fmt.Sprint(got) != "[session-001]" {
		t.Fatalf("project a still lists %v", got)
	}
	if got := namesIn(t, srv, b); len(got) != 1 {
		t.Fatalf("project b lists %v, want the moved conversation", got)
	}
	meta, _, _ := sessionstore.LoadBranchMeta(moved)
	if meta.Adopted == nil || meta.Adopted.Kind != "move" || meta.Adopted.From != paths[0] {
		t.Fatalf("the move is not recorded: %+v", meta.Adopted)
	}
}

func TestMovingRefusesWhatCannotBeMovedSafely(t *testing.T) {
	h, srv, a, b := twoProjects(t)
	paths := seedAged(t, a, 1)

	if status, _ := moveTo(t, srv, paths[0], testenv.TempDir(t)); status != http.StatusForbidden {
		t.Fatalf("a project the hub does not list = %d, want 403", status)
	}
	for name, target := range map[string]string{
		"a parent of a listed project":  filepath.Join(b, ".."),
		"a path that climbs out of one": filepath.Join(b, "..", "..", "etc"),
		"a path nothing has created":    filepath.Join(b, "no-such-dir"),
		"a relative path":               "../elsewhere",
	} {
		if status, out := moveTo(t, srv, paths[0], target); status != http.StatusForbidden || fmt.Sprint(out["code"]) != "workspace.unknown" {
			t.Fatalf("%s = %d %v, want 403 workspace.unknown", name, status, out)
		}
	}
	if status, _ := moveTo(t, srv, filepath.Join(testenv.TempDir(t), "x.jsonl"), b); status != http.StatusForbidden {
		t.Fatalf("a conversation outside every project = %d, want 403", status)
	}
	if status, out := moveTo(t, srv, paths[0], a); status != http.StatusConflict || fmt.Sprint(out["code"]) != "session.same_project" {
		t.Fatalf("its own project = %d %v, want 409 session.same_project", status, out)
	}
	if status, _ := moveTo(t, srv, paths[0], ""); status != http.StatusBadRequest {
		t.Fatalf("no target = %d, want 400", status)
	}
	lease, err := sessionstore.TryAcquireSessionLease(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	status, out := moveTo(t, srv, paths[0], b)
	lease.Release()
	if status != http.StatusConflict || !strings.HasPrefix(fmt.Sprint(out["code"]), "session.move_held") {
		t.Fatalf("a held conversation = %d %v, want 409 session.move_held", status, out)
	}
	if _, err := sessionstore.LoadSession(paths[0]); err != nil {
		t.Fatalf("the refused move touched the original: %v", err)
	}
	_ = h
}

func TestMovingRefusesAConversationAWindowHasOpen(t *testing.T) {
	h, srv, a, b := twoProjects(t)
	paths := seedAged(t, a, 1)
	for _, rt := range h.localRuntimes() {
		if filepath.Clean(rt.Server.Controller().WorkspaceRoot()) == filepath.Clean(a) {
			rt.Server.Controller().SetSessionPath(paths[0])
		}
	}
	status, out := moveTo(t, srv, paths[0], b)
	if status != http.StatusConflict || fmt.Sprint(out["code"]) != "session.move_open" {
		t.Fatalf("an open conversation = %d %v, want 409 session.move_open", status, out)
	}
}
