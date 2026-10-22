package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/platform/gitcmd"
	"reasonix/internal/platform/worktree"
	"reasonix/internal/state/sessionstore"
)

func projectWithWorktree(t *testing.T) (string, string) {
	t.Helper()
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	t.Setenv("REASONIX_STATE_HOME", home)
	root := testenv.TempDir(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "base", "--allow-empty"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	repo, err := gitcmd.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := worktree.Create(t.Context(), repo, config.DeliveryWorktreeDir())
	if err != nil {
		t.Fatal(err)
	}
	return repo.WorkTree, copy.WorkspaceRoot
}

func TestTreeGroupsManagedWorktreesWithoutMovingTranscripts(t *testing.T) {
	root, copy := projectWithWorktree(t)
	rememberWorkspace(root)
	rememberWorkspace(copy)
	local := seedAged(t, root, 3)
	isolated := seedAged(t, copy, 3)
	h := NewHub(HubOptions{})
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	rows := hubGet[[]treeWorkspace](t, srv, "/tree?limit=2")
	if len(rows) != 1 || rows[0].Root != root || rows[0].Isolated {
		t.Fatalf("worktrees must share their source project: %+v", rows)
	}
	if rows[0].Total != 6 || rows[0].Remaining != 4 || len(rows[0].Sessions) != 2 {
		t.Fatalf("one project-wide window: %+v", rows[0])
	}
	all := hubGet[[]treeWorkspace](t, srv, "/tree?limit=0")[0]
	for _, row := range all.Sessions {
		want := root
		if slices.Contains(isolated, row.Path) {
			want = copy
		} else if !slices.Contains(local, row.Path) {
			t.Fatalf("unknown row: %+v", row)
		}
		body, _ := json.Marshal(row)
		var wire map[string]any
		_ = json.Unmarshal(body, &wire)
		if wire["workspaceRoot"] != want {
			t.Errorf("session lost its working directory: %s", body)
		}
		if _, err := os.Stat(row.Path); err != nil {
			t.Fatal(err)
		}
	}
	// A restarted hub must group the existing list without a storage migration.
	other := NewHub(HubOptions{})
	res := httptest.NewRecorder()
	other.tree(res, httptest.NewRequest(http.MethodGet, "/tree", nil))
	var restarted []treeWorkspace
	_ = json.Unmarshal(res.Body.Bytes(), &restarted)
	if len(restarted) != 1 || restarted[0].Total != 6 {
		t.Fatalf("restart: %s", res.Body.String())
	}
}

func TestGroupedSessionOpensInItsOwnWorktreeForOldAndNewClients(t *testing.T) {
	root, copy := projectWithWorktree(t)
	rememberWorkspace(copy)
	path := seedAged(t, copy, 1)[0]
	h := NewHub(HubOptions{})
	for _, supplied := range []string{"", root, copy} {
		got, err := h.resolveRoot(OpenRequest{Root: supplied, SessionPath: path})
		if err != nil || got != copy {
			t.Errorf("root %q: opened %q (%v), want %q", supplied, got, err, copy)
		}
	}
}

func TestGroupedWorkspaceRemovalAndOrderingKeepAllMemberDirectories(t *testing.T) {
	root, copy := projectWithWorktree(t)
	other := testenv.TempDir(t)
	rememberWorkspace(root)
	rememberWorkspace(other)
	rememberWorkspace(copy)
	if err := placeWorkspace(context.Background(), root, other); err != nil {
		t.Fatal(err)
	}
	paths := Workspaces()
	if len(paths) != 3 || slices.Index(paths, copy) > slices.Index(paths, other) || slices.Index(paths, root) > slices.Index(paths, other) {
		t.Fatalf("the whole project must move together: %v", paths)
	}
	forgetWorkspace(root)
	if got := Workspaces(); !slices.Equal(got, []string{other}) {
		t.Fatalf("removed project retained a worktree: %v", got)
	}
}

func TestGroupedProjectFiltersAndSessionActionsKeepWorkingDirectories(t *testing.T) {
	root, copy := projectWithWorktree(t)
	local := seedAged(t, root, 2)
	isolated := seedAged(t, copy, 2)
	rememberWorkspace(copy)
	if err := sessionstore.SetSessionLineageArchived(isolated[1], true); err != nil {
		t.Fatal(err)
	}
	if err := sessionstore.SetSessionPinned(local[0], true); err != nil {
		t.Fatal(err)
	}
	if err := sessionstore.RenameSession(isolated[0], "isolated plan"); err != nil {
		t.Fatal(err)
	}
	h := NewHub(HubOptions{})
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	archived := treeFor(t, srv, "?scope=archived&limit=1")
	if archived.Total != 4 || archived.Archived != 1 || archived.Remaining != 0 || len(archived.Sessions) != 2 {
		t.Fatalf("archived row and pinned local row: %+v", archived)
	}
	query := treeFor(t, srv, "?q=isolated%20plan&limit=1")
	if len(query.Sessions) != 2 || query.Remaining != 0 {
		t.Fatalf("query must retain pinned rows from the other checkout: %+v", query)
	}
	for _, path := range []string{local[0], isolated[0]} {
		body := map[string]string{"path": path, "title": "renamed " + path}
		resp := postJSON(t, srv.URL+"/tree/sessions/rename", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			t.Fatalf("rename from grouped project: %d", resp.StatusCode)
		}
		resp = postExportSession(t, srv, path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("export from grouped project: %d", resp.StatusCode)
		}
	}
	location := hubGet[workspaceLocation](t, srv, "/host/workspaces/locate?root="+url.QueryEscape(root))
	if location.Path != root {
		t.Fatalf("original project cannot be revealed: %+v", location)
	}
	resp := postRemoveSession(t, srv, isolated[0])
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if _, err := os.Stat(local[0]); err != nil {
		t.Fatalf("deleting the same-named worktree session touched the original: %v", err)
	}
	if filepath.Base(local[0]) != filepath.Base(isolated[0]) {
		t.Fatal("fixture must exercise colliding transcript names")
	}
}

func TestWorktreeDoesNotConsumeAnotherProjectSlot(t *testing.T) {
	root, copy := projectWithWorktree(t)
	limitWorkspaces(t, 1)
	rememberWorkspace(root)
	if err := addRememberedWorkspace(t.Context(), copy); err != nil {
		t.Fatalf("a worktree consumed another project slot: %v", err)
	}
	if err := addRememberedWorkspace(t.Context(), testenv.TempDir(t)); err == nil {
		t.Fatal("an unrelated project bypassed the project limit")
	}
}

func TestWorktreeRegistrationPreservesTheOriginalProjectsPosition(t *testing.T) {
	root, copy := projectWithWorktree(t)
	other := testenv.TempDir(t)
	rememberWorkspace(root)
	rememberWorkspace(other)
	rememberWorkspace(copy)
	projects := rememberedProjects(Workspaces())
	if len(projects) != 2 || projects[0].root != other || projects[1].root != root {
		t.Fatalf("adding a checkout reordered its project: %+v", projects)
	}
	if got := LaunchWorkspaces()[0]; got != copy {
		t.Fatalf("launch checkout changed: %s", got)
	}
}

func TestGroupedHistoryReopensWithItsRuntimeAndKeepsNewChatsInSource(t *testing.T) {
	root, copy := projectWithWorktree(t)
	rememberWorkspace(copy)
	path := seedAged(t, copy, 1)[0]
	if err := os.WriteFile(config.UserConfigPath(), []byte(`default_model = "test/chat"
[[providers]]
name = "test"
kind = "openai"
base_url = "http://127.0.0.1:1/v1"
models = ["chat"]
default = "chat"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	closeSharedCatalogsOnCleanup(t)
	h := NewHub(HubOptions{})
	defer h.Shutdown()
	rt, err := h.Open(t.Context(), OpenRequest{Root: root, SessionPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if got := rt.Server.Controller().WorkspaceRoot(); got != copy {
		t.Fatalf("runtime opened at %s, want %s", got, copy)
	}
	again, err := h.Open(t.Context(), OpenRequest{Root: copy, SessionPath: path})
	if err != nil || again != rt {
		t.Fatalf("reopening must focus the existing runtime: %v", err)
	}
	if err := h.Close(rt.ID); err != nil {
		t.Fatal(err)
	}
	rt, err = h.Open(t.Context(), OpenRequest{Root: root, SessionPath: path})
	if err != nil || rt.Server.Controller().WorkspaceRoot() != copy {
		t.Fatalf("history lost its checkout after closing: %v", err)
	}
	fresh, err := h.Open(t.Context(), OpenRequest{Root: root})
	if err != nil || fresh.Server.Controller().WorkspaceRoot() != root {
		t.Fatalf("new conversation did not use the source directory: %v", err)
	}
}

func TestGroupedProjectRecognizesTheOriginalThroughASymlink(t *testing.T) {
	root, copy := projectWithWorktree(t)
	alias := filepath.Join(testenv.TempDir(t), "original")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	rememberWorkspace(alias)
	rememberWorkspace(copy)
	seedAged(t, alias, 1)
	seedAged(t, copy, 1)
	h := NewHub(HubOptions{})
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	rows := hubGet[[]treeWorkspace](t, srv, "/tree")
	if len(rows) != 1 || rows[0].Root != alias || rows[0].Total != 2 {
		t.Fatalf("source alias split its worktree: %+v", rows)
	}
	forgetWorkspace(alias)
	if got := Workspaces(); len(got) != 0 {
		t.Fatalf("source alias retained worktree bookmarks: %v", got)
	}
}
