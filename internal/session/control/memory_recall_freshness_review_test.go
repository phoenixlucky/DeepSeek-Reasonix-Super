package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/memory"
)

// A fact can change between the snapshot load and the turn that first hands it
// over: the digest taken at delivery time then describes bytes the turn never
// carried, and re-checking against it would call the snapshot body unchanged
// forever. Such a fact has to go out untracked so the identity read decides.
func TestRecallReloadsAFactEditedBeforeItsFirstRecall(t *testing.T) {
	root := testenv.TempDir(t)
	cwd := filepath.Join(root, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	userDir := filepath.Join(root, "user")
	// The same store the session will load, so the seeded fact is the one its
	// snapshot discovers.
	path, err := memory.StoreFor(userDir, cwd).Save(memory.Memory{
		Name:        "deploy-target",
		Description: "where the staging deploy goes",
		Keywords:    "staging deploy target",
		Type:        memory.TypeProject,
		Body:        "old body marker",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The snapshot is loaded with the fact's body as it reads now, and the file
	// is rewritten before anything has been delivered — the window the turn that
	// first hands the fact over has to survive.
	c := New(Options{Memory: memory.Load(memory.Options{CWD: cwd, UserDir: userDir})})
	rewriteOnDisk(t, path, "old body marker", "new body marker")

	if first := c.Compose("staging deploy target"); !strings.Contains(first, "old body marker") {
		t.Fatalf("turn 1 must carry the snapshot's (old) body for this test to answer anything:\n%s", first)
	}

	next := c.Compose("staging deploy target")
	if !strings.Contains(next, "new body marker") || strings.Contains(next, "old body marker") {
		t.Fatalf("the turn after a pre-first-recall edit still carries the stale snapshot body:\n%s", next)
	}
}

// A turn that recalls nothing must not erase what the previous turn handed the
// model: the fact is still standing of its own text, and an edit made after the
// quiet turn has to be seen. Only a recall that returned hits replaces the set.
func TestRecallKeepsCheckingAfterATurnWithNoHits(t *testing.T) {
	c, path := externalEditSession(t)
	if first := c.Compose("staging deploy target"); !strings.Contains(first, "old body marker") {
		t.Fatalf("setup: turn 1 does not carry the fact:\n%s", first)
	}
	if quiet := c.Compose("completely unrelated words about lunch"); strings.Contains(quiet, "old body marker") {
		t.Fatalf("the quiet turn was supposed to recall nothing:\n%s", quiet)
	}

	rewriteOnDisk(t, path, "old body marker", "new body marker")

	next := c.Compose("staging deploy target")
	if !strings.Contains(next, "new body marker") || strings.Contains(next, "old body marker") {
		t.Fatalf("a no-hit turn dropped the fact from the re-check set, so the edit was not seen:\n%s", next)
	}
}
