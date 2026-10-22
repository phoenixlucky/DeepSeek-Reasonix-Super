package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPeersAreOffUnlessTheUserFileTurnsThemOn(t *testing.T) {
	if Default().PeersEnabled() {
		t.Fatal("messages between sessions are on by default")
	}
	cfg, _ := loadScoped(t, "[peers]\nenabled = true\n", "")
	if !cfg.PeersEnabled() {
		t.Fatal("a user file's enabled = true was not honoured")
	}
}

// Turning this on opens a way for other sessions' text to reach this one, so it
// is the user's alone: a cloned repository can neither switch it on nor undo
// the user's choice.
func TestAProjectFileCannotSwitchPeersOn(t *testing.T) {
	cfg, _ := loadScoped(t, "", "[peers]\nenabled = true\n")
	if cfg.PeersEnabled() {
		t.Fatal("a cloned repo's reasonix.toml enabled messages between sessions")
	}
	if !slices.Contains(ignoredKeys(cfg), "peers.enabled") {
		t.Fatalf("ignored = %v, want peers.enabled reported", ignoredKeys(cfg))
	}
}

func TestAProjectFileCannotSwitchOffWhatTheUserChose(t *testing.T) {
	cfg, _ := loadScoped(t, "[peers]\nenabled = true\n", "[peers]\nenabled = false\n")
	if !cfg.PeersEnabled() {
		t.Fatal("the project file overrode the user's own peers setting")
	}
}

func TestSavingTheUserFileKeepsEveryPeersSetting(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	path := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[peers]\nenabled = true\ndispatch = true\nrelease_projects = [\"/work/b\", \"/work/c\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	cfg.Agent.MaxSideSessions = 2 // an unrelated save must not drop [peers]
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	got := LoadForEdit(path).Peers
	if !got.Enabled || !got.Dispatch || !slices.Equal(got.ReleaseProjects, []string{"/work/b", "/work/c"}) {
		t.Fatalf("peers after an unrelated save = %+v", got)
	}
}

func TestPeersDispatchNeedsPeersAndAProjectFileCannotGrantIt(t *testing.T) {
	cfg, _ := loadScoped(t, "[peers]\ndispatch = true\n", "")
	if cfg.PeersDispatch() {
		t.Fatal("dispatch worked without messages between sessions being on")
	}
	cfg, _ = loadScoped(t, "[peers]\nenabled = true\n", "[peers]\ndispatch = true\nrelease_projects = [\"/x\"]\n")
	if cfg.PeersDispatch() || cfg.PeersReleasedProject("/x") {
		t.Fatal("a cloned repo's reasonix.toml granted dispatch or a release")
	}
	for _, key := range []string{"peers.dispatch", "peers.release_projects"} {
		if !slices.Contains(ignoredKeys(cfg), key) {
			t.Fatalf("ignored = %v, want %s reported", ignoredKeys(cfg), key)
		}
	}
	cfg, _ = loadScoped(t, "[peers]\nenabled = true\ndispatch = true\nrelease_projects = [\"/x\"]\n", "")
	if !cfg.PeersDispatch() || !cfg.PeersReleasedProject("/x") || cfg.PeersReleasedProject("/y") {
		t.Fatalf("user file settings not honoured: %+v", cfg.Peers)
	}
}

func TestSavingPeersSettingsWritesThemAndReadsThemBack(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	path := UserConfigPath()
	cfg := LoadForEdit(path)
	cfg.Peers = PeersConfig{Enabled: true, Dispatch: true, ReleaseProjects: []string{"/work/b"}}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	got := LoadForEdit(path).Peers
	if !got.Enabled || !got.Dispatch || !slices.Equal(got.ReleaseProjects, []string{"/work/b"}) {
		t.Fatalf("peers after saving = %+v", got)
	}
	cfg = LoadForEdit(path)
	cfg.Peers = PeersConfig{}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadForEdit(path).Peers; got.Enabled || got.Dispatch || len(got.ReleaseProjects) != 0 {
		t.Fatalf("peers after clearing = %+v", got)
	}
}

func TestTheDispatchCeilingsDefaultToTheNamedValuesAndOnlyTheUserFileMovesThem(t *testing.T) {
	if d := Default(); d.PeersMaxDispatchDepth() != DefaultMaxDispatchDepth || d.PeersMaxDispatchedSessions() != DefaultMaxDispatchedSessions {
		t.Fatalf("defaults = %d, %d", d.PeersMaxDispatchDepth(), d.PeersMaxDispatchedSessions())
	}
	cfg, _ := loadScoped(t, "[peers]\nmax_dispatch_depth = 4\nmax_dispatched_sessions = 0\n", "")
	if cfg.PeersMaxDispatchDepth() != 4 || cfg.PeersMaxDispatchedSessions() != 0 {
		t.Fatalf("user file ceilings = %d, %d; want 4 and 0 (no ceiling)", cfg.PeersMaxDispatchDepth(), cfg.PeersMaxDispatchedSessions())
	}
	cfg, _ = loadScoped(t, "", "[peers]\nmax_dispatch_depth = 99\nmax_dispatched_sessions = 0\n")
	if cfg.PeersMaxDispatchDepth() != DefaultMaxDispatchDepth || cfg.PeersMaxDispatchedSessions() != DefaultMaxDispatchedSessions {
		t.Fatalf("a cloned repo moved a ceiling: %d, %d", cfg.PeersMaxDispatchDepth(), cfg.PeersMaxDispatchedSessions())
	}
	ignored := ignoredKeys(cfg)
	if !slices.Contains(ignored, "peers.max_dispatch_depth") || !slices.Contains(ignored, "peers.max_dispatched_sessions") {
		t.Fatalf("ignored = %v, want both ceilings reported", ignored)
	}
}

func TestCeilingsTheUserSetSurviveAnUnrelatedSave(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	path := UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[peers]\nmax_dispatch_depth = 0\nmax_dispatched_sessions = 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := LoadForEdit(path)
	cfg.Agent.MaxSideSessions = 2
	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}
	if got := LoadForEdit(path); got.PeersMaxDispatchDepth() != 0 || got.PeersMaxDispatchedSessions() != 3 {
		t.Fatalf("ceilings after a save = %d, %d; want 0 and 3", got.PeersMaxDispatchDepth(), got.PeersMaxDispatchedSessions())
	}
}
