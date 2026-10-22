package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/platform/openwith"
)

func TestAddWorktreeReturnsTheSelectedCheckoutWhileGroupingItsProject(t *testing.T) {
	root, copy := projectWithWorktree(t)
	h := NewHub(HubOptions{})
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	resp := postJSON(t, srv.URL+"/tree/workspaces", map[string]string{"path": copy})
	defer resp.Body.Close()
	var added treeWorkspace
	if err := json.NewDecoder(resp.Body).Decode(&added); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || added.Root != copy {
		t.Fatalf("the folder picker must open the selected checkout: status=%d, root=%q, want %q", resp.StatusCode, added.Root, copy)
	}
	rows := hubGet[[]treeWorkspace](t, srv, "/tree")
	if len(rows) != 1 || rows[0].Root != root {
		t.Fatalf("the selected checkout must still share its source project: %+v", rows)
	}
}

func TestSourceAliasKeepsPhysicalSourceAndAliasHistoryInTheGroupedProject(t *testing.T) {
	root, copy := projectWithWorktree(t)
	alias := filepath.Join(testenv.TempDir(t), "source-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	paths := []string{seedAged(t, root, 1)[0], seedAged(t, alias, 1)[0], seedAged(t, copy, 1)[0]}
	rememberWorkspace(copy)
	rememberWorkspace(alias)
	h := NewHub(HubOptions{})
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	rows := hubGet[[]treeWorkspace](t, srv, "/tree")
	if len(rows) != 1 || rows[0].Root != alias || rows[0].Total != 3 {
		t.Fatalf("choosing a source alias must not hide existing source history: %+v", rows)
	}
	for i, path := range paths {
		at := slices.IndexFunc(rows[0].Sessions, func(row treeSession) bool { return row.Path == path })
		if at < 0 {
			t.Fatalf("missing session %s", path)
		}
		want := []string{root, alias, copy}[i]
		if got, err := h.resolveRoot(OpenRequest{Root: alias, SessionPath: path}); err != nil || got != want {
			t.Fatalf("session checkout = %q (%v), want %q", got, err, want)
		}
		resp := postExportSession(t, srv, path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("grouped session cannot be exported: %d", resp.StatusCode)
		}
		resp = postJSON(t, srv.URL+"/tree/sessions/rename", map[string]string{"path": path, "title": "renamed"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			t.Fatalf("grouped session cannot be renamed: %d", resp.StatusCode)
		}
	}
}

func TestOpenWithProtectsImplicitSourceCheckoutForBothDiscoveryAndLaunch(t *testing.T) {
	root, copy := projectWithWorktree(t)
	rememberWorkspace(copy)
	calls := stubOpenWith(t, nil)
	var discoveryRoots []string
	openWithList = func(roots []string) []openwith.Status {
		discoveryRoots = slices.Clone(roots)
		return []openwith.Status{}
	}
	h := NewHub(HubOptions{})
	rec := httptest.NewRecorder()
	h.openWithTargets(rec, httptest.NewRequest(http.MethodGet, "/host/open-with", nil))
	if !slices.Contains(discoveryRoots, root) || !slices.Contains(discoveryRoots, copy) {
		t.Errorf("discovery may trust a program inside an implicit checkout: %v", discoveryRoots)
	}
	rec = postOpenWith(h, `{"id":"vscode","root":`+jsonString(root)+`}`, t.Context())
	if rec.Code != http.StatusOK || len(*calls) != 1 {
		t.Fatalf("cannot open grouped source: status=%d, calls=%+v", rec.Code, *calls)
	}
	if !slices.Contains((*calls)[0].forbidden, root) || !slices.Contains((*calls)[0].forbidden, copy) {
		t.Fatalf("launch may trust a program inside an implicit checkout: %v", (*calls)[0].forbidden)
	}
}

func TestGroupedProjectUsesExistingCheckoutWhenSourceFolderIsGone(t *testing.T) {
	for _, subdir := range []bool{false, true} {
		t.Run(map[bool]string{false: "repository moved", true: "selected source subfolder removed"}[subdir], func(t *testing.T) {
			root, copy := projectWithWorktree(t)
			if subdir {
				root, copy = filepath.Join(root, "app"), filepath.Join(copy, "app")
				for _, dir := range []string{root, copy} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			}
			seedAged(t, root, 1)
			seedAged(t, copy, 1)
			rememberWorkspace(copy)
			if err := os.RemoveAll(root); err != nil {
				t.Fatal(err)
			}
			h := NewHub(HubOptions{})
			srv := httptest.NewServer(operatorHandler(h))
			defer srv.Close()
			rows := hubGet[[]treeWorkspace](t, srv, "/tree")
			if len(rows) != 1 || rows[0].Missing || rows[0].Root != copy {
				t.Fatalf("the surviving checkout must remain usable: %+v", rows)
			}
			if got, err := h.resolveRoot(OpenRequest{Root: rows[0].Root}); err != nil || got != copy {
				t.Fatalf("new conversation uses unavailable source: %q (%v)", got, err)
			}
			if subdir && rows[0].Total != 2 {
				t.Fatalf("valid Git identity must retain both catalogs: %+v", rows[0])
			}
		})
	}
}
