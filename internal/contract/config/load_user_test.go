package config

import (
	"slices"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestLoadUserConfigReadOnlyIgnoresTheWorkingDirectoryProject(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "[tools.search]\nengine = \"native\"\n")

	project := testenv.TempDir(t)
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", "[tools.search]\nengine = \"rg\"\nrg_path = \"tools/rg\"\n\n[sandbox]\nbash = \"off\"\n")
	t.Chdir(project)

	merged, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if merged.Tools.Search.Engine != "rg" {
		t.Fatalf("precondition: merged load should read the project file, got %+v", merged.Tools.Search)
	}

	cfg, err := merged.Roots().LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tools.Search.Engine != "native" || cfg.Tools.Search.RgPath != "" {
		t.Fatalf("user-only search = %+v, want the user's native engine", cfg.Tools.Search)
	}
	if cfg.Sandbox.Bash == "off" {
		t.Fatal("user-only config took the project's sandbox mode")
	}
}

func TestLoadUserConfigReadOnlyFallsBackOnABrokenFile(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "[tools.search\n")

	cfg, err := LoadUserConfigReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tools.Search.RgPath != "" {
		t.Fatalf("broken user config should fall back to defaults, got %+v", cfg.Tools.Search)
	}
}

func TestProjectConfigCannotSetShellEnv(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "[tools.shell.env]\nCI = \"1\"\n")

	project := testenv.TempDir(t)
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", "[tools.shell.env]\nPATH = \"/repo/bin\"\nGIT_SSH_COMMAND = \"ssh -i /repo/key\"\n")
	t.Chdir(project)

	merged, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if merged.Tools.Shell.Env["CI"] != "1" {
		t.Fatalf("user shell env lost: %v", merged.Tools.Shell.Env)
	}
	if _, ok := merged.Tools.Shell.Env["PATH"]; ok {
		t.Fatalf("project injected PATH into shell env: %v", merged.Tools.Shell.Env)
	}
	if _, ok := merged.Tools.Shell.Env["GIT_SSH_COMMAND"]; ok {
		t.Fatalf("project injected GIT_SSH_COMMAND into shell env: %v", merged.Tools.Shell.Env)
	}
}

// A key that cannot be an environment entry, and a value carrying a NUL, must be
// reported at load: the bash tool drops the first, and the second fails exec.
func TestLoadWarnsAboutUnusableShellEnvEntries(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml",
		"[tools.shell.env]\n\"BAD=KEY\" = \"x\"\n\"\" = \"y\"\nNULVALUE = \"a\\u0000b\"\n")
	t.Chdir(testenv.TempDir(t))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cfg.LoadWarnings(), "\n")
	for _, want := range []string{"BAD=KEY", "empty key", "NULVALUE"} {
		if !strings.Contains(joined, want) {
			t.Errorf("load warnings lack %q:\n%s", want, joined)
		}
	}
}

// A project table is ignored, so the load has to say so rather than leave the
// user reading an effective config their reasonix.toml never reached.
func TestLoadWarnsWhenProjectSetsShellEnv(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "")

	project := testenv.TempDir(t)
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", "[tools.shell.env]\nCI = \"1\"\n")
	t.Chdir(project)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(cfg.LoadWarnings(), "\n"), "tools.shell") {
		t.Fatalf("no warning that the project shell env was ignored: %v", cfg.LoadWarnings())
	}
}

// Launcher arguments decide what program the agent's commands are handed to, so
// a repository can neither set them nor change the ones the user wrote.
func TestProjectConfigCannotSetOrChangeShellArgs(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "[tools.shell]\nprefer = \"bash\"\npath = \"/usr/bin/user-wsl\"\nargs = [\"-d\", \"mine\", \"--\", \"/bin/bash\"]\n")

	project := testenv.TempDir(t)
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", "[tools.shell]\nargs = [\"-d\", \"evil\", \"--\", \"/bin/sh\"]\n")
	t.Chdir(project)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"-d", "mine", "--", "/bin/bash"}; !slices.Equal(cfg.Tools.Shell.Args, want) {
		t.Fatalf("args = %q, want the user's %q", cfg.Tools.Shell.Args, want)
	}
	if !strings.Contains(strings.Join(cfg.LoadWarnings(), "\n"), "[tools.shell] args") {
		t.Fatalf("no warning that the project args were ignored: %v", cfg.LoadWarnings())
	}
}

func TestProjectConfigWithoutShellArgsLeavesTheUsersAlone(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	writeProjectDefaultTestConfig(t, home, "config.toml", "[tools.shell]\nargs = [\"-d\", \"mine\"]\n")
	project := testenv.TempDir(t)
	writeProjectDefaultTestConfig(t, project, "reasonix.toml", "[agent]\nsystem_prompt = \"x\"\n")
	t.Chdir(project)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Tools.Shell.Args, []string{"-d", "mine"}) {
		t.Fatalf("args = %q", cfg.Tools.Shell.Args)
	}
}

func TestShellArgsRoundTripThroughTheUserConfig(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	unlock := LockUserConfigEdits()
	defer unlock()
	orig := Default()
	if err := orig.SetShell("bash", "/usr/bin/user-wsl", "-d", `we"ird name`, "--", "/bin/bash"); err != nil {
		t.Fatal(err)
	}
	if err := orig.SaveTo(UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	got := LoadForEdit(UserConfigPath())
	if want := []string{"-d", `we"ird name`, "--", "/bin/bash"}; !slices.Equal(got.Tools.Shell.Args, want) {
		t.Fatalf("args = %q, want %q", got.Tools.Shell.Args, want)
	}
	if err := got.SetShell("auto", ""); err != nil {
		t.Fatal(err)
	}
	if len(got.Tools.Shell.Args) != 0 {
		t.Fatalf("choosing auto kept the arguments: %q", got.Tools.Shell.Args)
	}
}

func TestAProjectScopedRenderNeverWritesShellArgs(t *testing.T) {
	c := Default()
	if err := c.SetShell("bash", "/usr/bin/user-wsl", "-d", "x"); err != nil {
		t.Fatal(err)
	}
	if out := RenderTOMLForScope(c, RenderScopeProject); strings.Contains(out, "args") && strings.Contains(out, `"-d"`) {
		t.Fatalf("a project file would carry launcher arguments:\n%s", out)
	}
}
