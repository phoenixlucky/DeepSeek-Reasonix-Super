package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

// grantedFileServer is a workspace with one file of its own, a folder the user
// granted it (holding a text file), and a folder nobody granted.
func grantedFileServer(t *testing.T) (srv *httptest.Server, work, library, other string) {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	work, library, other = testenv.TempDir(t), testenv.TempDir(t), testenv.TempDir(t)
	for dir, files := range map[string]map[string]string{
		work:    {"own.md": "# own\n"},
		library: {"notes/plan.md": "# plan in the library\n"},
		other:   {"secret.md": "# not granted\n"},
	} {
		for name, body := range files {
			path := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: work})
	if err := config.NewProjectGrantStore(config.Roots{}.Home()).Update(work, func(g config.ProjectGrant) (config.ProjectGrant, error) {
		g.AllowWrite = append(g.AllowWrite, library)
		return g, nil
	}); err != nil {
		t.Fatal(err)
	}
	srv = httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)
	return srv, work, library, other
}

func readFile(t *testing.T, srv *httptest.Server, path, root string) (int, workspaceFile, Reason) {
	t.Helper()
	q := url.Values{"path": {path}}
	if root != "" {
		q.Set("root", root)
	}
	resp, err := http.Get(srv.URL + "/workspace/file?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var file workspaceFile
	var why Reason
	if resp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(resp.Body).Decode(&file)
	} else {
		_ = json.NewDecoder(resp.Body).Decode(&why)
	}
	return resp.StatusCode, file, why
}

func TestWorkspaceFileReadsFromAGrantedFolderReadOnly(t *testing.T) {
	srv, _, library, _ := grantedFileServer(t)
	status, file, _ := readFile(t, srv, "notes/plan.md", library)
	if status != http.StatusOK || file.Content != "# plan in the library\n" || !file.ReadOnly || file.Root != library {
		t.Fatalf("status %d, file %+v", status, file)
	}
	_, own, _ := readFile(t, srv, "own.md", "")
	if own.ReadOnly || own.Content != "# own\n" {
		t.Fatalf("the workspace's own file is not editable: %+v", own)
	}
}

func TestWorkspaceFileRefusesAFolderThatWasNotGranted(t *testing.T) {
	srv, _, _, other := grantedFileServer(t)
	status, _, why := readFile(t, srv, "secret.md", other)
	if status != http.StatusForbidden || why.Code != "workspace.root_not_granted" {
		t.Fatalf("status %d, reason %+v", status, why)
	}
}

// Naming a granted folder does not widen what a path may say: the path is
// still a name inside that folder, and the file is never reachable without it.
func TestWorkspaceFileGrantedFolderStaysConfined(t *testing.T) {
	srv, _, library, other := grantedFileServer(t)
	for _, path := range []string{"../" + filepath.Base(other) + "/secret.md", "../x.md", "/etc/hosts"} {
		if status, _, _ := readFile(t, srv, path, library); status != http.StatusBadRequest {
			t.Errorf("%q through a granted folder: status %d, want 400", path, status)
		}
	}
	if status, _, _ := readFile(t, srv, "notes/plan.md", ""); status != http.StatusNotFound {
		t.Fatalf("a granted folder's file read without naming it: status %d, want 404", status)
	}
}

func TestWorkspaceFileGrantedFolderRefusesASymlinkOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs a privilege on Windows")
	}
	srv, _, library, other := grantedFileServer(t)
	if err := os.Symlink(filepath.Join(other, "secret.md"), filepath.Join(library, "notes", "escape.md")); err != nil {
		t.Fatal(err)
	}
	if status, file, _ := readFile(t, srv, "notes/escape.md", library); status == http.StatusOK {
		t.Fatalf("a symlink in a granted folder read a file outside it: %+v", file)
	}
}

// A grant file is the user's, but it can be edited by hand or by something the
// user ran: a folder Reasonix keeps its own state or credentials in stays
// unreadable through this route even when a grant names it.
func TestWorkspaceFileRefusesAHandWrittenGrantOfAProtectedFolder(t *testing.T) {
	srv, work, _, _ := grantedFileServer(t)
	home := config.Roots{}.Home()
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "token.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.NewProjectGrantStore(home).Update(work, func(g config.ProjectGrant) (config.ProjectGrant, error) {
		g.AllowWrite = append(g.AllowWrite, home)
		return g, nil
	}); err != nil {
		t.Fatal(err)
	}
	status, file, why := readFile(t, srv, "token.txt", home)
	if status == http.StatusOK || file.Content != "" {
		t.Fatalf("read Reasonix's own state through a grant: status %d, %+v", status, file)
	}
	if status != http.StatusForbidden || why.Code != "workspace.root_not_granted" {
		t.Fatalf("status %d, reason %+v", status, why)
	}
}
