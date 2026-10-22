//go:build windows

package builtin

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/safety/sandbox"
)

// wslLauncher resolves the shell this machine's WSL gives, or skips: a runner
// without WSL has nothing to prove. REASONIX_TEST_WSL_DISTRO names the distro;
// its shell is whatever /bin/sh is there.
func wslLauncher(t *testing.T) (sandbox.Shell, func(command string) string) {
	t.Helper()
	distro := os.Getenv("REASONIX_TEST_WSL_DISTRO")
	wsl := filepath.Join(os.Getenv("SystemRoot"), "System32", "wsl.exe")
	if distro == "" {
		t.Skip("REASONIX_TEST_WSL_DISTRO is not set")
	}
	if _, err := os.Stat(wsl); err != nil {
		t.Skip("wsl.exe is not installed")
	}
	args := []string{"-d", distro, "--", "/bin/sh"}
	sh := sandbox.ShellDiscovery{Prefer: "bash", Path: wsl, Args: args}.Resolve()
	if len(sh.Args()) == 0 {
		t.Skipf("distro %q did not answer", distro)
	}
	inDistro := func(command string) string {
		out, _ := exec.Command(wsl, "-d", distro, "--", "/bin/sh", "-c", command).CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	return sh, inDistro
}

func TestBashRunsThroughAWSLLauncherByArgv(t *testing.T) {
	sh, _ := wslLauncher(t)
	if sh.Drives == "" || !strings.Contains(sh.Drives, `C:\ is /`) {
		t.Fatalf("the distro did not report where the drives are: %q", sh.Drives)
	}
	args, _ := json.Marshal(map[string]any{"command": "echo from-linux; uname -s; pwd"})
	out, err := (bash{shell: sh}).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("run: %v (%q)", err, out)
	}
	if !strings.Contains(out, "from-linux") || !strings.Contains(out, "Linux") {
		t.Fatalf("output = %q, want a Linux shell answering", out)
	}
	t.Logf("drives: %q; output: %q", sh.Drives, out)
}

// A cancelled command must end on the Linux side too: killing wsl.exe alone
// could leave the process running in the distro.
func TestBashCancelEndsTheLinuxProcessBehindWSL(t *testing.T) {
	sh, inDistro := wslLauncher(t)
	const marker = "31337.123"
	t.Cleanup(func() { inDistro("pkill -f '[s]leep " + marker + "' || true") })
	args, _ := json.Marshal(map[string]any{"command": "sleep " + marker})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := (bash{shell: sh}).Execute(ctx, args)
		done <- err
	}()
	started := false
	for range 50 {
		if inDistro("pgrep -f '[s]leep "+marker+"' | head -1") != "" {
			started = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !started {
		t.Fatal("the command never started in the distro")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(40 * time.Second):
		t.Fatal("cancel did not return within 40s")
	}
	for range 50 {
		if inDistro("pgrep -f '[s]leep "+marker+"' | head -1") == "" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the Linux process survived the cancel")
}
