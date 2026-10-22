package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// FakeLoginShell makes the login shell a command's PATH is probed from answer
// path at once, so a test neither spawns the user's real interactive shell nor
// depends on its startup files. Every probe form the host tries prints the marker.
func FakeLoginShell(t *testing.T, path string) {
	t.Helper()
	shell := filepath.Join(TempDir(t), "fake-login-shell")
	script := fmt.Sprintf("#!/bin/sh\nprintf '\\n__REASONIX_BASH_PATH__=%%s\\n' %q\n", path)
	if err := os.WriteFile(shell, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
}
