package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

type dirsFactory struct {
	fakeFactory
	mu     sync.Mutex
	params []SessionParams
	err    error
}

func (f *dirsFactory) NewSession(ctx context.Context, p SessionParams) (*control.Controller, error) {
	f.mu.Lock()
	f.params = append(f.params, p)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeFactory.NewSession(ctx, p)
}

func (f *dirsFactory) built() []SessionParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SessionParams(nil), f.params...)
}

// dirsWorld is a home, a workspace and a sibling folder. The sibling comes from
// the package directory: anything under the temp tree is a write root already.
func dirsWorld(t *testing.T) (home, cwd, sibling string) {
	t.Helper()
	base := testenv.TempDir(t)
	home, cwd = filepath.Join(base, "home"), filepath.Join(base, "ws")
	for _, d := range []string{home, cwd} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := os.MkdirTemp(".", "extra-*")
	if err != nil {
		t.Fatal(err)
	}
	if sibling, err = filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
	if sibling, err = filepath.EvalSymlinks(sibling); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sibling) })
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("REASONIX_HOME", filepath.Join(home, ".reasonix"))
	return home, cwd, sibling
}

func newWithDirs(t *testing.T, f Factory, cwd string, dirs ...string) frame {
	t.Helper()
	client, stop := startServer(t, f)
	t.Cleanup(stop)
	return client.call(t, "session/new", SessionNewParams{Cwd: cwd, AdditionalDirectories: dirs})
}

func refusalCode(t *testing.T, f frame) string {
	t.Helper()
	if f.Error == nil || f.Error.Code != ErrInvalidParams {
		t.Fatalf("want an invalid-params refusal, got %+v", f.Error)
	}
	var data AdditionalDirsData
	if err := json.Unmarshal(f.Error.Data, &data); err != nil {
		t.Fatalf("refusal data: %v", err)
	}
	return data.Code
}

func TestAdditionalDirectoriesReachTheSessionVettedAndStayOffTheUserGrants(t *testing.T) {
	_, cwd, sibling := dirsWorld(t)
	factory := &dirsFactory{}
	if resp := newWithDirs(t, factory, cwd, sibling, sibling); resp.Error != nil {
		t.Fatalf("session/new refused: %+v", resp.Error)
	}
	got := factory.built()
	real, _ := filepath.EvalSymlinks(sibling)
	if len(got) != 1 || len(got[0].AdditionalDirs) != 1 || got[0].AdditionalDirs[0] != real {
		t.Fatalf("the factory was handed %+v, want the one resolved folder %q", got, real)
	}
	grant, err := config.NewProjectGrantStore(config.Roots{}.Home()).Grant(cwd)
	if err != nil || len(grant.AllowWrite) != 0 {
		t.Fatalf("a client's folder reached the user's grants: %+v (%v)", grant, err)
	}
}

func TestAdditionalDirectoriesRefusedWholeWithTheFoldersOwnCode(t *testing.T) {
	home, cwd, sibling := dirsWorld(t)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(sibling, "link")
	if err := os.Symlink(filepath.Join(home, ".ssh"), link); err != nil {
		t.Skip("symlinks unavailable")
	}
	cases := []struct {
		name string
		dirs []string
		code string
	}{
		{"relative", []string{"../sibling"}, string(config.RootNotDirectory)},
		{"missing", []string{filepath.Join(sibling, "nope")}, string(config.RootNotDirectory)},
		{"the workspace itself", []string{cwd}, string(config.RootCoveredAlready)},
		{"a key folder", []string{filepath.Join(home, ".ssh")}, string(config.RootProtected)},
		{"a link to a key folder", []string{link}, string(config.RootProtected)},
		{"one good, one bad", []string{sibling, filepath.Join(home, ".ssh")}, string(config.RootProtected)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			factory := &dirsFactory{}
			if got := refusalCode(t, newWithDirs(t, factory, cwd, c.dirs...)); got != c.code {
				t.Fatalf("code = %q, want %q", got, c.code)
			}
			if n := len(factory.built()); n != 0 {
				t.Fatalf("a refused request still built %d session(s)", n)
			}
		})
	}
}

func TestAdditionalDirectoriesAreCapped(t *testing.T) {
	_, cwd, sibling := dirsWorld(t)
	dirs := make([]string, maxAdditionalDirectories+1)
	for i := range dirs {
		dirs[i] = sibling
	}
	if got := refusalCode(t, newWithDirs(t, &dirsFactory{}, cwd, dirs...)); got != codeAdditionalDirsTooMany {
		t.Fatalf("code = %q", got)
	}
}

func TestAnAgentThatWritesOnlyInTheCwdSaysSoInsteadOfIgnoringTheFolders(t *testing.T) {
	_, cwd, sibling := dirsWorld(t)
	factory := &dirsFactory{err: ErrAdditionalDirectoriesDisabled}
	if got := refusalCode(t, newWithDirs(t, factory, cwd, sibling)); got != codeAdditionalDirsOff {
		t.Fatalf("code = %q", got)
	}
}

func TestResumingAnOpenSessionWithOtherFoldersIsRefusedNotMerged(t *testing.T) {
	_, cwd, sibling := dirsWorld(t)
	client, stop := startServer(t, &dirsFactory{})
	t.Cleanup(stop)
	resp := client.call(t, "session/new", SessionNewParams{Cwd: cwd, AdditionalDirectories: []string{sibling}})
	var nr SessionNewResult
	if err := json.Unmarshal(resp.Result, &nr); err != nil || nr.SessionID == "" {
		t.Fatalf("session/new: %+v", resp.Error)
	}
	same := client.call(t, "session/resume", SessionResumeParams{SessionID: nr.SessionID, Cwd: cwd, AdditionalDirectories: []string{sibling}})
	if same.Error != nil {
		t.Fatalf("resuming with the same folders: %+v", same.Error)
	}
	other := client.call(t, "session/resume", SessionResumeParams{SessionID: nr.SessionID, Cwd: cwd})
	if got := refusalCode(t, other); got != codeAdditionalDirsChanged {
		t.Fatalf("code = %q", got)
	}
}

func TestInitializeAdvertisesAdditionalDirectories(t *testing.T) {
	client, stop := startServer(t, &dirsFactory{})
	t.Cleanup(stop)
	resp := client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	var res struct {
		AgentCapabilities struct {
			SessionCapabilities map[string]json.RawMessage `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatal(err)
	}
	if _, ok := res.AgentCapabilities.SessionCapabilities["additionalDirectories"]; !ok {
		t.Fatalf("capabilities = %v", res.AgentCapabilities.SessionCapabilities)
	}
}

type capturingRebuilder struct {
	*configurableFactory
	got SessionParams
}

func (f *capturingRebuilder) RebuildSession(_ context.Context, p SessionParams, _ *control.Controller) (*control.Controller, error) {
	f.got = p
	return control.New(control.Options{Label: "rebuilt"}), nil
}

func TestReloadingExtensionsKeepsTheSessionsAdditionalDirectories(t *testing.T) {
	notifier := &fakeNotifier{}
	sess := reloadExtensionsSession(t, "sess-dirs", control.New(control.Options{}), notifier)
	sess.additionalDirs = []string{"/work/other"}
	factory := &capturingRebuilder{configurableFactory: &configurableFactory{}}
	svc := &service{factory: factory, sessions: map[string]*acpSession{sess.id: sess}}
	if _, err := svc.sessionReloadExtensions(context.Background(), marshalReloadParams(t, sess.id)); err != nil {
		t.Fatal(err)
	}
	if len(factory.got.AdditionalDirs) != 1 || factory.got.AdditionalDirs[0] != "/work/other" {
		t.Fatalf("the rebuilt controller was given %v", factory.got.AdditionalDirs)
	}
}
