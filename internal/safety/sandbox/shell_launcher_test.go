package sandbox

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

// fakeLauncher behaves like `wsl.exe -d <distro> -- <shell>`: it only answers
// when given exactly that prefix, then runs what follows. Run with no
// arguments it fails, the way the bare launcher does.
func fakeLauncher(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script launcher")
	}
	launchersSupported = true
	t.Cleanup(func() { launchersSupported = runtime.GOOS == "windows" })
	launcherProofs.Clear()
	path := filepath.Join(testenv.TempDir(t), "fake-wsl")
	script := "#!/bin/sh\n[ \"$1\" = -d ] || exit 3\nshift 2\n[ \"$1\" = -- ] || exit 4\nshift\nexec \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLauncherWithItsArgumentsIsProvenByRunningThem(t *testing.T) {
	path := fakeLauncher(t)
	if _, err := VerifyLauncher("bash", path, nil); err == nil {
		t.Fatal("the bare launcher does not run a command and must not verify")
	}
	args := []string{"-d", "kali", "--", "/bin/sh"}
	proof, err := VerifyLauncher("bash", path, args)
	if err != nil {
		t.Fatal(err)
	}
	if want := append([]string{path}, args...); !slices.Equal(proof.Argv, want) {
		t.Fatalf("argv shown = %q, want %q", proof.Argv, want)
	}
	var warn bytes.Buffer
	sh := ShellDiscovery{Prefer: "bash", Path: path, Args: args, Warn: &warn}.Resolve()
	if sh.Path != path || !slices.Equal(sh.Args(), args) || sh.Kind != ShellBash || warn.Len() != 0 {
		t.Fatalf("resolved %+v with warnings %q", sh, warn.String())
	}
	got := sh.argv("echo hi")
	want := []string{path, "-d", "kali", "--", "/bin/sh", "-c", "echo hi"}
	if !slices.Equal(got, want) {
		t.Fatalf("command argv = %q, want %q", got, want)
	}
}

func TestLauncherRunsArgumentsByArgvNotThroughAShell(t *testing.T) {
	path := fakeLauncher(t)
	marker := filepath.Join(testenv.TempDir(t), "injected")
	args := []string{"-d", "x; touch " + marker, "--", "/bin/sh"}
	if _, err := VerifyLauncher("bash", path, args); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("an argument was interpreted by a shell")
	}
}

func TestUnusableLauncherSettingsFallBackToAutoWithAReason(t *testing.T) {
	path := fakeLauncher(t)
	cases := []struct {
		name         string
		prefer, path string
		args         []string
		why          string
	}{
		{"wrong arguments", "bash", path, []string{"-x"}, "did not run a command"},
		{"not bash", "powershell", path, []string{"-d", "x", "--", "/bin/sh"}, `prefer = "bash"`},
		{"no path", "bash", "", []string{"-d", "x"}, "need a path"},
		{"relative path", "bash", "wsl.exe", []string{"-d", "x"}, "absolute path"},
		{"empty argument", "bash", path, []string{"-d", "", "--"}, "empty"},
		{"line break", "bash", path, []string{"-d", "a\nb"}, "line breaks"},
		{"missing program", "bash", filepath.Join(testenv.TempDir(t), "nope"), []string{"-d", "x"}, "no such executable"},
	}
	for _, c := range cases {
		var warn bytes.Buffer
		sh := ShellDiscovery{Prefer: c.prefer, Path: c.path, Args: c.args, Warn: &warn}.Resolve()
		if len(sh.Args()) != 0 || sh.Path == path && c.name != "not bash" {
			t.Errorf("%s: kept the launcher: %+v", c.name, sh)
		}
		if !strings.Contains(warn.String(), c.why) || !strings.Contains(warn.String(), "auto-detection") {
			t.Errorf("%s: warning = %q, want it to say %q and the fallback", c.name, warn.String(), c.why)
		}
	}
}

func TestLauncherArgumentsAreWindowsOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows supports them")
	}
	launcherProofs.Clear()
	var warn bytes.Buffer
	sh := ShellDiscovery{Prefer: "bash", Path: "/bin/sh", Args: []string{"-d", "x"}, Warn: &warn}.Resolve()
	if len(sh.Args()) != 0 || !strings.Contains(warn.String(), "only supported on Windows") {
		t.Fatalf("resolved %+v with %q", sh, warn.String())
	}
}

func TestShellWithoutArgsKeepsItsArgv(t *testing.T) {
	sh := Shell{Kind: ShellBash, Path: "/bin/bash"}
	if got, want := sh.argv("ls"), []string{"/bin/bash", "-c", "ls"}; !slices.Equal(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

// A launcher that hangs (a distro that will not start) costs one bounded wait and
// is then refused; it never wedges settings or startup.
func TestHungLauncherIsRefusedWithinTheBound(t *testing.T) {
	path := fakeLauncher(t)
	hang := filepath.Join(filepath.Dir(path), "hang")
	if err := os.WriteFile(hang, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcherProbeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { launcherProbeTimeout = bashProbeTimeout })
	start := time.Now()
	_, err := VerifyLauncher("bash", hang, []string{"-d", "x"})
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("err = %v, want a timeout refusal", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the refusal took %v; the bound is not holding", took)
	}
}

func TestLauncherShellIsDescribedForTheModelWithItsRealMounts(t *testing.T) {
	plain := Shell{Kind: ShellBash, Path: "/bin/bash"}
	if plain.Description() != "" {
		t.Fatalf("a plain shell needs no description, got %q", plain.Description())
	}
	wsl := Shell{Kind: ShellBash, Path: `C:\Windows\System32\wsl.exe`, Drives: `C:\ is /mnt/host/c/`}.WithArgs([]string{"-d", "Ubuntu", "--", "/bin/bash"})
	want := `C:\Windows\System32\wsl.exe -d Ubuntu -- /bin/bash (Linux under WSL; C:\ is /mnt/host/c/)`
	if got := wsl.Description(); got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
	other := Shell{Kind: ShellBash, Path: "/opt/launcher"}.WithArgs([]string{"run"})
	if got := other.Description(); got != "/opt/launcher run" {
		t.Fatalf("description = %q", got)
	}
}
