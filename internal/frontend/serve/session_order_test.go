package serve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/sessionstore"
)

func info(path string, at int, rank float64) sessionstore.SessionInfo {
	return sessionstore.SessionInfo{Path: path, LastActivityAt: time.Unix(int64(at), 0), ManualRank: rank}
}

func paths(order []sessionstore.SessionInfo) []string {
	out := make([]string, len(order))
	for i, s := range order {
		out[i] = s.Path
	}
	return out
}

func TestUnplacedSessionsComeFirstByActivityThenPlacedOnesByRank(t *testing.T) {
	list := []sessionstore.SessionInfo{info("old-placed-2", 1, 2048), info("new-unplaced", 9, 0), info("placed-1", 5, 1024), info("older-unplaced", 3, 0)}
	sortSessions(list)
	want := []string{"new-unplaced", "older-unplaced", "placed-1", "old-placed-2"}
	for i, p := range paths(list) {
		if p != want[i] {
			t.Fatalf("order = %v, want %v", paths(list), want)
		}
	}
}

func TestMovingASessionPlacesEveryUnplacedOneOnceAndKeepsTheVisibleOrder(t *testing.T) {
	list := []sessionstore.SessionInfo{info("a", 9, 0), info("b", 8, 0), info("c", 7, 0)}
	sortSessions(list)
	got, ok := rankUpdatesForStep(list, "c", -1)
	if !ok || len(got) != 3 {
		t.Fatalf("first placement ranks the whole list: %v %v", got, ok)
	}
	for i := range list {
		list[i].ManualRank = got[list[i].Path]
	}
	sortSessions(list)
	if w := []string{"a", "c", "b"}; paths(list)[0] != w[0] || paths(list)[1] != w[1] || paths(list)[2] != w[2] {
		t.Fatalf("after moving c up the order is %v, want %v", paths(list), w)
	}
}

func TestLaterMovesWriteOnlyTheMovedSessionWhileThereIsRoom(t *testing.T) {
	list := []sessionstore.SessionInfo{info("a", 1, 1024), info("b", 1, 2048), info("c", 1, 3072)}
	got, ok := rankUpdatesForStep(list, "c", -1)
	if !ok || len(got) != 1 || got["c"] != 1536 {
		t.Fatalf("moving c between a and b: %v %v", got, ok)
	}
	got, _ = rankUpdatesForStep(list, "a", 1)
	if len(got) != 1 || got["a"] != 2560 {
		t.Fatalf("moving a between b and c: %v", got)
	}
	got, _ = rankUpdatesForStep(list, "b", 1)
	if len(got) != 1 || got["b"] != 4096 {
		t.Fatalf("moving b to the end: %v", got)
	}
}

func TestRanksAreRenumberedWhenTheGapRunsOut(t *testing.T) {
	list := []sessionstore.SessionInfo{info("a", 1, 1024), info("b", 1, 1024.0005), info("c", 1, 4096)}
	got, ok := rankUpdatesForStep(list, "c", -1)
	if !ok || len(got) < 2 {
		t.Fatalf("a closed gap renumbers more than the moved one: %v %v", got, ok)
	}
	for i := range list {
		if rank, changed := got[list[i].Path]; changed {
			list[i].ManualRank = rank
		}
	}
	sortSessions(list)
	if p := paths(list); p[0] != "a" || p[1] != "c" || p[2] != "b" {
		t.Fatalf("after the renumbering the order is %v, want a c b", p)
	}
}

func TestAStepAtTheEdgeOrOnAnUnknownSessionChangesNothing(t *testing.T) {
	list := []sessionstore.SessionInfo{info("a", 1, 1024), info("b", 1, 2048)}
	if got, ok := rankUpdatesForStep(list, "a", -1); ok || got != nil {
		t.Fatalf("a is already first: %v %v", got, ok)
	}
	if got, ok := rankUpdatesForStep(list, "b", 1); ok || got != nil {
		t.Fatalf("b is already last: %v %v", got, ok)
	}
	if _, ok := rankUpdatesForStep(list, "nope", 1); ok {
		t.Fatal("an unknown session has no place to move")
	}
}

func TestOnlyConversationsInTheSameStateAreNeighbours(t *testing.T) {
	arch := info("archived", 5, 1024)
	arch.Archived = true
	list := []sessionstore.SessionInfo{info("a", 9, 2048), arch, info("b", 1, 3072)}
	sortSessions(list)
	got, ok := rankUpdatesForStep(list, "b", -1)
	if !ok || got["b"] >= 2048 {
		t.Fatalf("moving b up passes a, not the archived row between them: %v %v", got, ok)
	}
	if _, touched := got["archived"]; touched {
		t.Fatalf("a row that was not part of the move was rewritten: %v", got)
	}
}

func moveSessionRequest(t *testing.T, srv *httptest.Server, path string, direction int) *http.Response {
	t.Helper()
	resp, err := http.Post(srv.URL+"/tree/sessions/move", "application/json", strings.NewReader(fmt.Sprintf(`{"path":%q,"direction":%d}`, path, direction)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func rowNames(ws treeWorkspace) []string {
	out := make([]string, len(ws.Sessions))
	for i, s := range ws.Sessions {
		out[i] = s.Name
	}
	return out
}

func TestMovingASessionChangesTheTreeAndSurvivesAReload(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	paths := seedAged(t, root, 4)

	if got := rowNames(treeFor(t, srv, "")); fmt.Sprint(got) != "[session-003 session-002 session-001 session-000]" {
		t.Fatalf("by activity: %v", got)
	}
	if resp := moveSessionRequest(t, srv, paths[0], -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("move = %d", resp.StatusCode)
	}
	if got := rowNames(treeFor(t, srv, "")); fmt.Sprint(got) != "[session-003 session-002 session-000 session-001]" {
		t.Fatalf("after moving the oldest up one: %v", got)
	}
	if resp := moveSessionRequest(t, srv, paths[3], 1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("move = %d", resp.StatusCode)
	}
	if got := rowNames(treeFor(t, srv, "")); fmt.Sprint(got) != "[session-002 session-003 session-000 session-001]" {
		t.Fatalf("after moving the newest down one: %v", got)
	}
	if resp := moveSessionRequest(t, srv, paths[2], -1); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("an edge step is not an error: %d", resp.StatusCode)
	}
	if got := rowNames(treeFor(t, srv, "")); fmt.Sprint(got) != "[session-002 session-003 session-000 session-001]" {
		t.Fatalf("an edge step changed the order: %v", got)
	}
}

func TestAConversationCreatedAfterPlacingStartsOnTop(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	paths := seedAged(t, root, 3)
	moveSessionRequest(t, srv, paths[0], -1)
	fresh := filepath.Join(SessionDirFor(root), "session-new.jsonl")
	writeSessionAt(t, fresh)
	if got := rowNames(treeFor(t, srv, "")); got[0] != "session-new" {
		t.Fatalf("a conversation nobody placed belongs on top, got %v", got)
	}
}

func TestMovingRefusesAPathOutsideEveryWorkspaceAndABadDirection(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	hubRuntime(t, h, root)
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	seedAged(t, root, 2)
	if resp := moveSessionRequest(t, srv, filepath.Join(testenv.TempDir(t), "elsewhere.jsonl"), 1); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a path outside the workspaces = %d, want 403", resp.StatusCode)
	}
	inside := filepath.Join(SessionDirFor(root), "session-000.jsonl")
	if resp := moveSessionRequest(t, srv, inside, 2); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("direction 2 = %d, want 400", resp.StatusCode)
	}
}
