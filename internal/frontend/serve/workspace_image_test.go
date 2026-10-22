package serve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func imageServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	dir := testenv.TempDir(t)
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: dir})
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)
	return srv
}

func TestWorkspaceImageServesAnImageTheDocumentReferences(t *testing.T) {
	srv := imageServer(t, map[string]string{"docs/img/logo.png": "\x89PNG fake"})
	resp, err := http.Get(srv.URL + "/workspace/image?path=docs/img/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || string(body) != "\x89PNG fake" {
		t.Fatalf("status %d, type %q, body %q", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}

// An SVG opened at this address is a document on the kernel's origin; the
// sandbox is what keeps the script it may carry from running there.
func TestWorkspaceImageSandboxesWhatItServes(t *testing.T) {
	srv := imageServer(t, map[string]string{"a.svg": `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`})
	resp, err := http.Get(srv.URL + "/workspace/image?path=a.svg")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("CSP = %q, want a sandbox with nothing allowed by default", csp)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("the type must not be sniffed")
	}
}

func TestWorkspaceImageRefusesWhatIsNotAnImageInTheTree(t *testing.T) {
	srv := imageServer(t, map[string]string{"notes.html": "<script>1</script>", "a.png": "x"})
	cases := map[string]int{
		"notes.html":     http.StatusBadRequest,
		"../outside.png": http.StatusBadRequest,
		"missing.png":    http.StatusNotFound,
		"%2e%2e/etc.png": http.StatusBadRequest,
		"/etc/x.png":     http.StatusBadRequest,
	}
	// A drive letter only makes a path absolute on Windows; elsewhere
	// "C:/Windows/x.png" is a relative name, and not finding it is a 404.
	if runtime.GOOS == "windows" {
		cases["C:/Windows/x.png"] = http.StatusBadRequest
	}
	for path, want := range cases {
		resp, err := http.Get(srv.URL + "/workspace/image?path=" + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: status %d, want %d", path, resp.StatusCode, want)
		}
	}
}

// A folder the user granted this workspace can show its pictures too: the
// grant is the authority, named by its own path, and nothing else is reachable.
func TestWorkspaceImageServesFromAGrantedFolderOnly(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	work := testenv.TempDir(t)
	library := testenv.TempDir(t)
	other := testenv.TempDir(t)
	for dir, name := range map[string]string{library: "wall/moon.png", other: "secret.png"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("\x89PNG "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: work})
	granted := library
	if err := config.NewProjectGrantStore(config.Roots{}.Home()).Update(work, func(g config.ProjectGrant) (config.ProjectGrant, error) {
		g.AllowWrite = append(g.AllowWrite, library)
		return g, nil
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)

	get := func(root, path string) (int, string) {
		u := srv.URL + "/workspace/image?path=" + url.QueryEscape(path)
		if root != "" {
			u += "&root=" + url.QueryEscape(root)
		}
		resp, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	if code, body := get(granted, "wall/moon.png"); code != http.StatusOK || !strings.Contains(body, "wall/moon.png") {
		t.Fatalf("a granted folder's picture: %d %q", code, body)
	}
	if code, _ := get(other, "secret.png"); code != http.StatusForbidden {
		t.Fatalf("a folder nobody granted: status %d, want 403", code)
	}
	if code, _ := get(granted, "../"+filepath.Base(other)+"/secret.png"); code == http.StatusOK {
		t.Fatalf("a path climbing out of the granted folder was served")
	}
}
