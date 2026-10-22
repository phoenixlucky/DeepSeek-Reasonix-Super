package cli

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

// `serve --dir` must switch into the named project before it resolves anything
// from the working directory, as `run`, the terminal UI and `subagent` do. The
// check drives the command, not the two helpers: a test that called them
// directly would pass with the bug still in place. `--hash-password` prints and
// returns before a server starts, so exit 0 means the directory was taken first.
func TestServeDirSwitchesBeforeItResolvesAnything(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Restore before TempDir's cleanup removes the directory (t.Cleanup is LIFO):
	// Windows cannot delete a directory that is still the process working
	// directory.
	t.Cleanup(func() { _ = os.Chdir(orig) })

	project := testenv.TempDir(t)

	var code int
	captureStderr(t, func() {
		captureStdout(t, func() {
			code = runServeWithOptions(
				[]string{"--dir", project, "--hash-password", "--password", "x"},
				serveRunOptions{command: "serve"},
			)
		})
	})
	if code != 0 {
		t.Fatalf("serve --dir %s exited %d, want 0", project, code)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !sameDir(cwd, project) {
		t.Fatalf("serve --dir %s left the working directory at %q", project, cwd)
	}
}

// A --dir that cannot be entered is refused, and refuses the command: the two
// answers are different places to work, and continuing in the current directory
// would file the session and the write scope somewhere nobody asked for.
func TestServeDirRefusesWhatItCannotEnter(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	missing := filepath.Join(testenv.TempDir(t), "does-not-exist")

	var code int
	stderr := captureStderr(t, func() {
		captureStdout(t, func() {
			code = runServeWithOptions(
				[]string{"--dir", missing, "--hash-password", "--password", "x"},
				serveRunOptions{command: "serve"},
			)
		})
	})
	if code != 2 {
		t.Fatalf("serve --dir %s exited %d, want 2", missing, code)
	}
	if stderr == "" {
		t.Fatal("a refused --dir said nothing on stderr")
	}
	cwd, _ := os.Getwd()
	if !sameDir(cwd, orig) {
		t.Fatalf("a refused --dir moved the working directory to %q", cwd)
	}
}

func sameDir(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}
