package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// TestRememberToolSaves drives the tool the way the agent does — raw JSON args —
// and verifies the fact lands in the store and the index.
func TestRememberToolSaves(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	tl := NewRememberTool(store)

	if tl.Name() != "remember" || tl.ReadOnly() {
		t.Fatalf("unexpected tool identity: name=%q readonly=%v", tl.Name(), tl.ReadOnly())
	}
	// Schema must be valid JSON the provider can forward.
	if !json.Valid(tl.Schema()) {
		t.Fatal("remember schema is not valid JSON")
	}

	args := []byte(`{"name":"likes-go","title":"Likes Go","description":"User likes Go","type":"user","scope":"global","body":"Default to Go for backend work."}`)
	out, err := tl.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Saved memory") {
		t.Fatalf("unexpected tool output: %q", out)
	}
	if strings.Contains(out, store.Dir) || !strings.Contains(out, "global/likes-go.md") {
		t.Fatalf("tool output must use a stable reference, got %q", out)
	}

	list := mustStore(t, store).List()
	if len(list) != 1 || list[0].Name != "likes-go" || list[0].Type != TypeUser || list[0].Scope != FactScopeGlobal {
		t.Fatalf("memory not saved correctly: %+v", list)
	}
	if list[0].Title != "Likes Go" {
		t.Fatalf("title not persisted through the tool: %q", list[0].Title)
	}
}

func TestRememberToolDefaultsToProjectScope(t *testing.T) {
	root := testenv.TempDir(t)
	store := Store{Dir: root + "/project", GlobalDir: root + "/global"}
	if _, err := NewRememberTool(store).Execute(context.Background(), []byte(`{"name":"project-feedback","description":"current project only","type":"feedback","body":"body"}`)); err != nil {
		t.Fatal(err)
	}
	list := mustStore(t, store).List()
	if len(list) != 1 || list[0].Scope != FactScopeProject {
		t.Fatalf("memory = %+v, want project scope", list)
	}
}

func TestRememberToolUpdateWithoutScopePreservesLegacyGlobalScope(t *testing.T) {
	root := testenv.TempDir(t)
	store := Store{Dir: filepath.Join(root, "project"), GlobalDir: filepath.Join(root, "global")}
	if err := os.MkdirAll(store.GlobalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "---\nname: no-emoji\ndescription: legacy global feedback\nmetadata:\n  type: feedback\n---\n\nAvoid emoji.\n"
	globalPath := filepath.Join(store.GlobalDir, "no-emoji.md")
	if err := os.WriteFile(globalPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := reindexIn(store.GlobalDir, "no-emoji", Memory{Name: "no-emoji", Description: "legacy global feedback", Type: TypeFeedback, Scope: FactScopeGlobal}); err != nil {
		t.Fatal(err)
	}

	out, err := NewRememberTool(store).Execute(context.Background(), []byte(`{"name":"no-emoji","description":"updated global feedback","type":"feedback","body":"Avoid emoji in every project."}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "(global background)") {
		t.Fatalf("tool output did not report inherited global scope: %q", out)
	}
	if _, err := os.Stat(globalPath); err != nil {
		t.Fatalf("global memory was not kept active: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "no-emoji.md")); !os.IsNotExist(err) {
		t.Fatalf("omitted-scope update created a project copy, stat err=%v", err)
	}
	list := mustStore(t, store).List()
	if len(list) != 1 || list[0].Scope != FactScopeGlobal || !strings.Contains(list[0].Body, "every project") {
		t.Fatalf("updated memory = %+v, want one global active copy", list)
	}
}

func TestRememberToolUpdatesScopeQualifiedReference(t *testing.T) {
	root := testenv.TempDir(t)
	store := Store{Dir: filepath.Join(root, "project"), GlobalDir: filepath.Join(root, "global")}
	if _, err := store.SaveWithOptions(Memory{Name: "project/shared.md", Description: "project", Body: "project body"}, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWithOptions(Memory{Name: "global/shared.md", Description: "global", Body: "global body"}, SaveOptions{}); err != nil {
		t.Fatal(err)
	}

	out, err := NewRememberTool(store).Execute(context.Background(), []byte(`{"name":"global/shared.md","expected_revision":1,"description":"updated global","body":"global v2"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "global/shared.md") || strings.Contains(out, root) {
		t.Fatalf("qualified update output = %q", out)
	}
	global, ok := mustStore(t, store).Read("global/shared.md")
	if !ok || global.Name != "shared" || global.Scope != FactScopeGlobal || global.Revision != 2 || global.Body != "global v2" {
		t.Fatalf("updated global = %+v, ok=%v", global, ok)
	}
	project, ok := mustStore(t, store).Read("project/shared.md")
	if !ok || project.Revision != 1 || project.Body != "project body" {
		t.Fatalf("project fact changed = %+v, ok=%v", project, ok)
	}
}

// A name active in two scopes must come back to the model carrying the
// refusal's identity, both candidate ids, and the two ways to say which fact
// was meant. The tool returns the store's error, which the agent renders into
// the tool result the model reads.
func TestRememberToolReturnsCrossScopeNameIdentity(t *testing.T) {
	store, global, project := crossScopeNameStore(t)

	out, err := NewRememberTool(store).Execute(context.Background(), []byte(`{"name":"shared","description":"updated","body":"v2"}`))
	if err == nil {
		t.Fatalf("remember reported success for a name active in two scopes: %q", out)
	}
	if out != "" {
		t.Fatalf("a refused remember returned an output body: %q", out)
	}
	if !errors.Is(err, ErrNameActiveInTwoScopes) {
		t.Fatalf("errors.Is(err, ErrNameActiveInTwoScopes) = false (err = %v)", err)
	}
	for _, want := range []string{
		"memory.name_active_in_two_scopes",
		global.Memory.ID,
		project.Memory.ID,
		"stable id",
		"scope-qualified",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the model-facing refusal does not render %q:\n%s", want, err)
		}
	}
}

// TestRememberToolValidates rejects calls missing required fields rather than
// writing an empty memory.
func TestRememberToolValidates(t *testing.T) {
	tl := NewRememberTool(Store{Dir: testenv.TempDir(t)})
	if _, err := tl.Execute(context.Background(), []byte(`{"description":"d"}`)); err == nil {
		t.Fatal("expected error when body is missing")
	}
	if _, err := tl.Execute(context.Background(), []byte(`{"body":"b"}`)); err == nil {
		t.Fatal("expected error when description is missing")
	}
	if _, err := tl.Execute(context.Background(), []byte(`{"description":"d","body":"b","scope":"workspace"}`)); err == nil {
		t.Fatal("expected error for an unknown scope")
	}
}

// TestRememberToolQueuesNote verifies a save injects a turn-tail note so the
// fact applies this session, not only the next.
func TestRememberToolQueuesNote(t *testing.T) {
	q := &fakeQueue{}
	ctx := WithQueue(context.Background(), q)
	tl := NewRememberTool(Store{Dir: testenv.TempDir(t)})
	if _, err := tl.Execute(ctx, []byte(`{"name":"uses-rmb","description":"balance is RMB","type":"user","body":"b"}`)); err != nil {
		t.Fatal(err)
	}
	if len(q.notes) != 1 || !strings.Contains(q.notes[0], "uses-rmb") || !strings.Contains(q.notes[0], "\nb") {
		t.Fatalf("expected one queued note with the saved memory name and body, got %v", q.notes)
	}
}

// TestRememberToolQueuesTheTypedFact pins the queue contract: the fact the tool
// wrote rides the queue as a typed Memory, so the controller records what was
// written rather than parsing the note's prose back into memory.
func TestRememberToolQueuesTheTypedFact(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	q := &fakeQueue{}
	ctx := WithQueue(context.Background(), q)
	if _, err := NewRememberTool(store).Execute(ctx, []byte(`{"name":"uses-rmb","description":"balance is RMB","type":"user","body":"b"}`)); err != nil {
		t.Fatal(err)
	}
	if len(q.delivered) != 1 {
		t.Fatalf("remember queued %d typed facts, want exactly 1", len(q.delivered))
	}
	saved, ok := mustStore(t, store).Read("uses-rmb")
	if !ok {
		t.Fatal("the tool wrote no fact")
	}
	got := q.delivered[0]
	if got.ID != saved.ID || got.Revision != saved.Revision || got.Body != saved.Body ||
		got.Name != saved.Name || got.Scope != saved.Scope {
		t.Fatalf("queued typed fact %+v does not describe the saved fact %+v", got, saved)
	}
}

// TestForgetToolQueuesNoTypedFact: a delete has nothing left to re-check, so it
// must not hand the controller a delivered fact to chase.
func TestForgetToolQueuesNoTypedFact(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	if _, err := store.Save(Memory{Name: "old-fact", Description: "d", Type: TypeProject, Body: "b"}); err != nil {
		t.Fatal(err)
	}
	q := &fakeQueue{}
	ctx := WithQueue(context.Background(), q)
	if _, err := NewForgetTool(store).Execute(ctx, []byte(`{"name":"old-fact"}`)); err != nil {
		t.Fatal(err)
	}
	if len(q.delivered) != 0 {
		t.Fatalf("forget queued %d typed facts, want none: %+v", len(q.delivered), q.delivered)
	}
}

func TestRememberToolQueuesResolvedNameWhenUpdatingByID(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "stable-name", Description: "before", Body: "before"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	q := &fakeQueue{}
	ctx := WithQueue(context.Background(), q)
	args := []byte(`{"id":"` + first.Memory.ID + `","expected_revision":1,"description":"after","body":"after"}`)
	if _, err := NewRememberTool(store).Execute(ctx, args); err != nil {
		t.Fatal(err)
	}
	if len(q.notes) != 1 || !strings.Contains(q.notes[0], "stable-name") {
		t.Fatalf("queued note did not use the resolved memory name: %v", q.notes)
	}
}
