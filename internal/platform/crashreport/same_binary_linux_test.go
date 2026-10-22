//go:build linux

package crashreport

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// A pid that names no process cannot be this program, and asking must not guess.
func TestSameBinaryRejectsNonPids(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if sameBinary(pid) {
			t.Fatalf("sameBinary(%d) = true, want false: it names no process", pid)
		}
	}
}

// A live pid belonging to a different program must not read as this one, or its
// dump file is never cleared.
func TestSameBinaryRejectsAPidThatIsAnotherProgram(t *testing.T) {
	other := otherProcess(t)
	if sameBinary(other) {
		t.Fatalf("sameBinary(%d) = true, but that pid belongs to another program; its file would never be cleared", other)
	}
}

// A second instance of this program under a bare argv[0] is still this program.
// argv[0] is the name the caller passed — an instance started through PATH
// carries "reasonix", which no path resolves — and reading that as the identity
// clears a live sibling's dump out from under it.
func TestSameBinaryAcceptsThisProgramUnderABareName(t *testing.T) {
	pid := sameBinaryUnderBareName(t)
	if !sameBinary(pid) {
		t.Fatalf("sameBinary(%d) = false, but that pid is this program under a bare argv[0]; its dump would be cleared while it runs", pid)
	}
}

const sameBinaryChildEnv = "REASONIX_TEST_SAME_BINARY_CHILD"

// sameBinaryUnderBareName starts a second copy of this binary, reached the way a
// PATH lookup reaches it — argv[0] is a bare name — and waits until it is there.
func sameBinaryUnderBareName(t *testing.T) int {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestSameBinaryChild$")
	child.Args[0] = "reasonix"
	child.Env = append(os.Environ(), sameBinaryChildEnv+"=1")
	if err := child.Start(); err != nil {
		t.Skipf("cannot start a second instance of this binary: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
	})
	pid := child.Process.Pid
	if pid == os.Getpid() {
		t.Skip("the second instance was given this one's pid")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe")); err == nil {
			return pid
		}
		if time.Now().After(deadline) {
			t.Skipf("pid %d never reported its exe", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSameBinaryChild is the body of that second instance, not a case of its own.
func TestSameBinaryChild(t *testing.T) {
	if os.Getenv(sameBinaryChildEnv) == "" {
		t.Skip("this is the parent; the child holds this test open")
	}
	time.Sleep(60 * time.Second)
}
