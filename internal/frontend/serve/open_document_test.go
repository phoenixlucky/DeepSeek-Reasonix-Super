package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

type documentFixture struct {
	srv                  *Server
	work, library, other string
	launched             []string
	launchErr            error
}

func newDocumentFixture(t *testing.T, granted bool) *documentFixture {
	t.Helper()
	return newDocumentFixtureAt(t, granted, false)
}

// aliased runs the kernel with its workspace named through a symlink, so the
// root as written and the root as it resolves are different strings.
func newDocumentFixtureAt(t *testing.T, granted, aliased bool) *documentFixture {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	f := &documentFixture{work: testenv.TempDir(t), library: testenv.TempDir(t), other: testenv.TempDir(t)}
	for dir, names := range map[string][]string{
		f.work:    {"site/index.html", "site/app.js", "site/run.exe", "site/note.txt", "page.HTML", "pdf/a.pdf", "pic.svg"},
		f.library: {"docs/guide.html"},
		f.other:   {"secret.html"},
	} {
		for _, name := range names {
			path := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	root := f.work
	if aliased {
		root = filepath.Join(testenv.TempDir(t), "alias")
		if err := os.Symlink(f.work, root); err != nil {
			t.Fatal(err)
		}
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: root})
	t.Cleanup(ctrl.Close)
	if err := config.NewProjectGrantStore(config.Roots{}.Home()).Update(f.work, func(g config.ProjectGrant) (config.ProjectGrant, error) {
		g.AllowWrite = append(g.AllowWrite, f.library)
		return g, nil
	}); err != nil {
		t.Fatal(err)
	}
	f.srv = New(ctrl, bc, config.ServeConfig{})
	if granted {
		f.srv.AllowLocalDesktop()
	}
	prev := launchDocument
	launchDocument = func(path string) error { f.launched = append(f.launched, path); return f.launchErr }
	t.Cleanup(func() { launchDocument = prev })
	return f
}

func (f *documentFixture) open(t *testing.T, path string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"path": path})
	rec := httptest.NewRecorder()
	f.srv.openDocument(rec, httptest.NewRequest(http.MethodPost, "/workspace/open-document", strings.NewReader(string(body))))
	var env struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env.Code
}

func TestOpenDocumentIsRefusedWithoutTheDesktopGrant(t *testing.T) {
	f := newDocumentFixture(t, false)
	code, why := f.open(t, filepath.Join(f.work, "site", "index.html"))
	if code != http.StatusForbidden || why != codeDocumentNoWindow || len(f.launched) != 0 {
		t.Fatalf("status %d code %q launched %v; want 403 %s and nothing started", code, why, f.launched, codeDocumentNoWindow)
	}
}

func TestOpenDocumentStartsAnAllowedFileInsideTheWorkspaceOrAGrantedFolder(t *testing.T) {
	f := newDocumentFixture(t, true)
	for _, path := range []string{
		filepath.Join(f.work, "site", "index.html"),
		filepath.Join(f.work, "page.HTML"),
		filepath.Join(f.work, "pdf", "a.pdf"),
		filepath.Join(f.work, "pic.svg"),
		filepath.Join(f.library, "docs", "guide.html"),
		"site/index.html",
	} {
		before := len(f.launched)
		if code, why := f.open(t, path); code != http.StatusOK {
			t.Errorf("%s: status %d code %q, want 200", path, code, why)
			continue
		}
		if len(f.launched) != before+1 || !filepath.IsAbs(f.launched[before]) {
			t.Errorf("%s: launched %v, want one absolute path", path, f.launched)
		}
	}
}

func TestOpenDocumentRefusesWhatIsOutsideTheRoots(t *testing.T) {
	f := newDocumentFixture(t, true)
	for _, path := range []string{
		filepath.Join(f.other, "secret.html"),
		filepath.Join(f.work, "..", filepath.Base(f.other), "secret.html"),
		"../" + filepath.Base(f.other) + "/secret.html",
	} {
		if code, why := f.open(t, path); code != http.StatusForbidden || why != codeDocumentOutsideRoots {
			t.Errorf("%s: status %d code %q, want 403 %s", path, code, why, codeDocumentOutsideRoots)
		}
	}
	if len(f.launched) != 0 {
		t.Fatalf("something outside the roots was started: %v", f.launched)
	}
}

func TestOpenDocumentRefusesTypesOffTheListAndWhatIsNotAFile(t *testing.T) {
	f := newDocumentFixture(t, true)
	for _, name := range []string{"site/app.js", "site/run.exe"} {
		if code, why := f.open(t, filepath.Join(f.work, filepath.FromSlash(name))); code != http.StatusBadRequest || why != codeDocumentType {
			t.Errorf("%s: status %d code %q, want 400 %s", name, code, why, codeDocumentType)
		}
	}
	if code, why := f.open(t, filepath.Join(f.work, "site")); code != http.StatusBadRequest || why != codeDocumentNotFile {
		t.Errorf("a folder: status %d code %q, want 400 %s", code, why, codeDocumentNotFile)
	}
	if code, why := f.open(t, filepath.Join(f.work, "site", "missing.html")); code != http.StatusNotFound || why != codeDocumentMissing {
		t.Errorf("a missing file: status %d code %q, want 404 %s", code, why, codeDocumentMissing)
	}
	if len(f.launched) != 0 {
		t.Fatalf("a refused file was started: %v", f.launched)
	}
}

// The type that counts is the one the file really has once its links are
// followed, and a link may not lead out of the roots.
func TestOpenDocumentJudgesTheResolvedFileNotTheLinksName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs a privilege on Windows")
	}
	f := newDocumentFixture(t, true)
	if err := os.Symlink(filepath.Join(f.work, "site", "run.exe"), filepath.Join(f.work, "innocent.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.other, "secret.html"), filepath.Join(f.work, "escape.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.work, "site", "index.html"), filepath.Join(f.work, "inside.html")); err != nil {
		t.Fatal(err)
	}
	if code, why := f.open(t, filepath.Join(f.work, "innocent.html")); code != http.StatusBadRequest || why != codeDocumentType {
		t.Errorf("a .html link to an .exe: status %d code %q, want 400 %s", code, why, codeDocumentType)
	}
	if code, why := f.open(t, filepath.Join(f.work, "escape.html")); code != http.StatusForbidden || why != codeDocumentOutsideRoots {
		t.Errorf("a link out of the roots: status %d code %q, want 403 %s", code, why, codeDocumentOutsideRoots)
	}
	if len(f.launched) != 0 {
		t.Fatalf("a refused link was started: %v", f.launched)
	}
	if code, _ := f.open(t, filepath.Join(f.work, "inside.html")); code != http.StatusOK || len(f.launched) != 1 {
		t.Fatalf("a link to an allowed file inside the roots: status %d launched %v", code, f.launched)
	}
	if got := f.launched[0]; !strings.HasSuffix(filepath.ToSlash(got), "/site/index.html") {
		t.Errorf("the opener was handed %q, want the resolved file", got)
	}
}

func TestOpenDocumentReportsAFailedLaunch(t *testing.T) {
	f := newDocumentFixture(t, true)
	f.launchErr = os.ErrPermission
	if code, why := f.open(t, filepath.Join(f.work, "site", "index.html")); code != http.StatusInternalServerError || why != codeDocumentLaunch {
		t.Fatalf("status %d code %q, want 500 %s", code, why, codeDocumentLaunch)
	}
}

func TestOpenDocumentAcceptsAPathSpelledThroughALinkedWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs a privilege on Windows")
	}
	f := newDocumentFixtureAt(t, true, true)
	real, err := filepath.EvalSymlinks(f.work)
	if err != nil {
		t.Fatal(err)
	}
	alias := f.srv.ctl().WorkspaceRoot()
	for _, path := range []string{
		filepath.Join(alias, "site", "index.html"),
		filepath.Join(real, "site", "index.html"),
		"site/index.html",
	} {
		before := len(f.launched)
		if code, why := f.open(t, path); code != http.StatusOK || len(f.launched) != before+1 {
			t.Errorf("%s: status %d code %q launched %v, want 200 and one launch", path, code, why, f.launched)
		}
	}
	if code, why := f.open(t, filepath.Join(f.other, "secret.html")); code != http.StatusForbidden || why != codeDocumentOutsideRoots {
		t.Errorf("outside the roots with an aliased workspace: status %d code %q", code, why)
	}
}
