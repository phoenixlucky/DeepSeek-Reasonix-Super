package memory

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

// The two cases the single-identity audit found outside its own scope: a name/id
// collision across scopes, and the legacy same-name other-scope archive path.
// They are reproduced here from the audit that found them.

// crossScopeNameStore seeds one fact named "shared" in each scope: the state an
// unqualified save cannot resolve to a single identity.
func crossScopeNameStore(t *testing.T) (Store, SaveResult, SaveResult) {
	t.Helper()
	root := testenv.TempDir(t)
	store := Store{Dir: filepath.Join(root, "project"), GlobalDir: filepath.Join(root, "global")}
	global, err := store.SaveWithOptions(Memory{Name: "global/shared.md", Description: "global", Body: "global body"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.SaveWithOptions(Memory{Name: "project/shared.md", Description: "project", Body: "project body"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return store, global, project
}

// An unqualified update whose name is already active in more than one scope has
// no single identity to update: guessing would archive a fact the caller never
// named, so the store must refuse and say which facts collide.
func TestStoreV2UnqualifiedUpdateRefusesADuplicateNameAcrossScopes(t *testing.T) {
	root := testenv.TempDir(t)
	store := Store{Dir: filepath.Join(root, "project"), GlobalDir: filepath.Join(root, "global")}
	if _, err := store.SaveWithOptions(Memory{Name: "global/shared.md", Description: "global", Body: "global body"}, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWithOptions(Memory{Name: "project/shared.md", Description: "project", Body: "project body"}, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	before, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}

	_, saveErr := store.SaveWithOptions(Memory{Name: "shared", Description: "updated", Body: "project v2"}, SaveOptions{})
	if saveErr == nil {
		t.Fatal("an unqualified update of a name active in two scopes reported success")
	}
	if !strings.Contains(saveErr.Error(), "shared") {
		t.Fatalf("the refusal does not name the colliding name: %v", saveErr)
	}
	after, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("the refused update changed the store: %d active facts before, %d after", len(before), len(after))
	}
	for _, m := range after {
		if m.Body == "project v2" {
			t.Fatalf("the refused update was written anyway: %+v", m)
		}
	}
}

// A caller has to tell this refusal apart from a write failure by identity, not
// by reading the sentence, and the sentence has to hand back both ids so the
// caller knows which two facts the name could have meant.
func TestStoreV2CrossScopeNameRefusalCarriesIdentityAndBothIDs(t *testing.T) {
	store, global, project := crossScopeNameStore(t)

	_, saveErr := store.SaveWithOptions(Memory{Name: "shared", Description: "updated", Body: "v2"}, SaveOptions{})
	if !errors.Is(saveErr, ErrNameActiveInTwoScopes) {
		t.Fatalf("errors.Is(err, ErrNameActiveInTwoScopes) = false (err = %v); a caller cannot tell a cross-scope name collision from a write failure", saveErr)
	}
	for _, want := range []string{
		"memory.name_active_in_two_scopes",
		global.Memory.ID,
		project.Memory.ID,
		"stable id",
		"scope-qualified",
	} {
		if !strings.Contains(saveErr.Error(), want) {
			t.Fatalf("the refusal does not render %q:\n%s", want, saveErr)
		}
	}
}

// The refusal is total — both facts keep their own bytes and revision — while
// the two unambiguous ways to say which fact was meant still save.
func TestStoreV2CrossScopeNameRefusalLeavesBothFactsSaveable(t *testing.T) {
	store, global, project := crossScopeNameStore(t)

	if _, saveErr := store.SaveWithOptions(Memory{Name: "shared", Description: "updated", Body: "unqualified v2"}, SaveOptions{}); saveErr == nil {
		t.Fatal("an unqualified update of a name active in two scopes reported success")
	}
	active := func(t *testing.T, label, globalBody, projectBody string, globalRevision, projectRevision int) {
		t.Helper()
		for _, want := range []struct {
			id       string
			body     string
			revision int
		}{
			{global.Memory.ID, globalBody, globalRevision},
			{project.Memory.ID, projectBody, projectRevision},
		} {
			got, ok, err := store.Read(want.id)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("%s: id %s is no longer active", label, want.id)
			}
			if got.Body != want.body || got.Revision != want.revision {
				t.Fatalf("%s: id %s = revision %d body %q, want revision %d body %q",
					label, want.id, got.Revision, got.Body, want.revision, want.body)
			}
		}
	}
	active(t, "after the refusal", "global body", "project body", 1, 1)
	archived, err := store.ListArchived()
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("the refused update archived a fact the caller never named: %+v", archived)
	}

	if _, err := store.SaveWithOptions(Memory{ID: global.Memory.ID, Description: "global updated", Body: "global v2"}, SaveOptions{}); err != nil {
		t.Fatalf("a save by stable id was refused: %v", err)
	}
	active(t, "after the id save", "global v2", "project body", 2, 1)

	if _, err := store.SaveWithOptions(Memory{Name: "project/shared.md", Description: "project updated", Body: "project v2"}, SaveOptions{}); err != nil {
		t.Fatalf("a scope-qualified save was refused: %v", err)
	}
	active(t, "after the qualified save", "global v2", "project v2", 2, 2)
}

// The refusal is about an unqualified name that resolves to two facts, so a
// name active in exactly one scope must still update it rather than be refused.
func TestStoreV2CrossScopeRefusalOnlyFiresWhenBothScopesHoldTheName(t *testing.T) {
	root := testenv.TempDir(t)
	store := Store{Dir: filepath.Join(root, "project"), GlobalDir: filepath.Join(root, "global")}
	global, err := store.SaveWithOptions(Memory{Name: "global/only-here.md", Description: "global", Body: "global body"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.SaveWithOptions(Memory{Name: "only-here", Description: "updated", Body: "global v2"}, SaveOptions{}); err != nil {
		t.Fatalf("an unqualified update of a name active in one scope was refused: %v", err)
	}
	got, ok, err := store.Read(global.Memory.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got.Body != "global v2" || got.Revision != 2 {
		t.Fatalf("id %s = %+v, want revision 2 body %q", global.Memory.ID, got, "global v2")
	}
	facts, err := store.ListAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("the unqualified update created a second fact: %+v", facts)
	}
}

// A failed rename must not leave a revision that a later stale write can be
// accepted against by accident of the directory listing order.
func TestStoreV2FailedRenameLeavesNoAcceptableStaleRevision(t *testing.T) {
	for _, tc := range []struct{ oldName, newName string }{
		{"alpha", "beta"},
		{"zeta", "alpha"},
	} {
		t.Run(tc.oldName+"_to_"+tc.newName, func(t *testing.T) {
			store := Store{Dir: testenv.TempDir(t)}
			first, err := store.SaveWithOptions(Memory{Name: tc.oldName, Description: "first", Body: "v1"}, SaveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			failRenames(t, func(path string) bool { return strings.HasSuffix(path, tc.oldName+".md") })

			if _, saveErr := store.SaveWithOptions(Memory{
				ID: first.Memory.ID, Name: tc.newName, Description: "renamed", Body: "v2",
			}, SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}); saveErr == nil {
				t.Fatal("the rename reported success although the old name could not be retired")
			}
			stopFailing(store.Dir)

			// Whatever is on disk now, only one identity may claim this id, and a
			// revision-1 write must be judged against it rather than against a
			// file the directory listing happened to return first.
			var actives []Memory
			all, err := store.ListAll()
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range all {
				if m.ID == first.Memory.ID {
					actives = append(actives, m)
				}
			}
			if len(actives) > 1 {
				t.Fatalf("the failed rename left %d active identities for one id: %+v", len(actives), actives)
			}
			if len(actives) == 1 && actives[0].Revision != 1 {
				t.Fatalf("the failed rename left revision %d on disk: %+v", actives[0].Revision, actives[0])
			}
		})
	}
}
