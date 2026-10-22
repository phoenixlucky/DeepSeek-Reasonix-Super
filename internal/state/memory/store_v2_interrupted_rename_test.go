package memory

// Probes for a rename killed between retiring the old name and publishing the
// new one: the hidden copies publishRename leaves behind must not make the fact
// permanently unreachable.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/fileutil"
	"reasonix/internal/base/testenv"
)

// killedAtBoundary is the panic value used to emulate an abrupt termination.
type killedAtBoundary struct{}

// killDuring runs fn with the shared crash hook armed, and stops fn at the
// first persistence boundary match accepts. Panicking (rather than returning an
// error) leaves exactly the bytes a SIGKILL would leave.
func killDuring(t *testing.T, match func(op, path string) bool, fn func()) bool {
	t.Helper()
	previous := fileutil.CrashPoint
	fileutil.CrashPoint = func(op, path string) {
		if match(op, path) {
			panic(killedAtBoundary{})
		}
	}
	killed := false
	func() {
		defer func() {
			fileutil.CrashPoint = previous
			if r := recover(); r != nil {
				if _, ok := r.(killedAtBoundary); !ok {
					panic(r)
				}
				killed = true
			}
		}()
		fn()
	}()
	return killed
}

// isIndexPublish reports a persistence boundary that writes MEMORY.md.
func isIndexPublish(op, path string) bool {
	return (op == "atomic-write" || op == "replace") && filepath.Base(path) == indexFile
}

// A rename retires the old name before publishing the new one (so no failure
// can leave one id with two active names). A kill inside that window leaves the
// only copy of the fact under a hidden .retired-* name: no reader lists it, and
// the retry that carries the id the caller still holds cannot find it.
func TestAuditKillBetweenRetireAndPublishMakesTheFactUnreachable(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	intent := Memory{ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2"}
	opts := SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}

	killed := killDuring(t, isIndexPublish, func() {
		_, _ = store.SaveWithOptions(intent, opts)
	})
	if !killed {
		t.Fatal("the probe never reached the index publication, so the window was not exercised")
	}

	_, oldOK := mustStore(t, store).Read("alpha")
	_, newOK := mustStore(t, store).Read("beta")
	_, idOK := mustStore(t, store).Read(first.Memory.ID)
	list, listErr := store.List()
	revisions, revisionsErr := store.Revisions(first.Memory.ID)
	retried, retryErr := store.SaveWithOptions(intent, opts)
	byName, byNameErr := store.SaveWithOptions(Memory{Name: "beta", Description: "renamed", Body: "v2"}, SaveOptions{})

	retired, _ := filepath.Glob(filepath.Join(store.Dir, ".retired-*"))
	staged, _ := filepath.Glob(filepath.Join(store.Dir, ".staged-*"))
	snapshot := filepath.Join(store.Dir, ".revisions", first.Memory.ID, "000000001.md")
	snapshotBody := ""
	if raw, err := os.ReadFile(snapshot); err == nil {
		snapshotBody = string(raw)
	}
	if oldOK || newOK || idOK || retryErr != nil || len(list) != 0 {
		t.Fatalf("a kill between retiring the old name and publishing the new one left the fact unreachable:\n"+
			"  Read(alpha)=%v Read(beta)=%v Read(id=%s)=%v\n"+
			"  List()=%d facts, err=%v\n"+
			"  retry by id = %v (revision %d)\n"+
			"  retry by name = err=%v id=%q (same id: %v), so a name-based retry keeps the word but mints a second identity\n"+
			"  Revisions(id)=%d facts, err=%v — the history read also needs an active fact first\n"+
			"  hidden copies on disk: retired=%v staged=%v\n"+
			"  pre-rename snapshot %s still holds v1: %v\n"+
			"  no reader, repair path, or retry names any of these, so the bytes are only recoverable by hand",
			oldOK, newOK, first.Memory.ID, idOK, len(list), listErr, retryErr, retried.Memory.Revision,
			byNameErr, byName.Memory.ID, byName.Memory.ID == first.Memory.ID,
			len(revisions), revisionsErr, retired, staged, snapshot, strings.Contains(snapshotBody, "v1"))
	}
}

// A rename that is not interrupted must be byte-for-byte the old behaviour:
// one active identity, no hidden copies, one index line.
func TestRenameWithoutKillLeavesOneActiveIdentity(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := store.SaveWithOptions(Memory{
		ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2",
	}, SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true})
	if err != nil {
		t.Fatal(err)
	}
	oldFact, oldOK := mustStore(t, store).Read("alpha")
	newFact, newOK := mustStore(t, store).Read("beta")
	byID, idOK := mustStore(t, store).Read(first.Memory.ID)
	names := namesInListForID(t, store, first.Memory.ID)
	index := mustReadString(t, filepath.Join(store.Dir, indexFile))
	retired, _ := filepath.Glob(filepath.Join(store.Dir, ".retired-*"))
	staged, _ := filepath.Glob(filepath.Join(store.Dir, ".staged-*"))
	if oldOK || !newOK || !idOK || newFact.ID != first.Memory.ID || renamed.Memory.Revision != 2 ||
		len(names) != 1 || names[0] != "beta" || strings.Count(index, "beta.md") != 1 ||
		strings.Contains(index, "alpha.md") || len(retired) != 0 || len(staged) != 0 {
		t.Fatalf("the uninterrupted rename changed:\n"+
			"  Read(alpha)=%+v oldOK=%v\n"+
			"  Read(beta)=%+v newOK=%v\n"+
			"  Read(id)=%+v idOK=%v\n"+
			"  active names for the id: %v (revision %d)\n"+
			"  hidden copies: retired=%v staged=%v\n"+
			"  MEMORY.md =\n%s",
			oldFact, oldOK, newFact, newOK, byID, idOK, names, renamed.Memory.Revision, retired, staged, index)
	}
}

// After the interrupted rename is recovered, a retry by name must update the
// recovered identity instead of minting a second one with the same name.
func TestNameRetryAfterRecoveredRenameKeepsTheSameID(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	intent := Memory{ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2"}
	opts := SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}
	if killed := killDuring(t, isIndexPublish, func() { _, _ = store.SaveWithOptions(intent, opts) }); !killed {
		t.Fatal("the probe never reached the index publication, so the window was not exercised")
	}
	if _, err := store.SaveWithOptions(intent, opts); err != nil {
		t.Fatalf("the retry by id did not recover the interrupted rename: %v", err)
	}
	byName, err := store.SaveWithOptions(Memory{Name: "beta", Description: "renamed", Body: "v3"}, SaveOptions{})
	if err != nil {
		t.Fatalf("the retry by name failed: %v", err)
	}
	names := namesInListForID(t, store, first.Memory.ID)
	if byName.Memory.ID != first.Memory.ID || len(names) != 1 || names[0] != "beta" {
		t.Fatalf("a name retry after recovery minted a second identity:\n"+
			"  save by name returned id %q (recovered id %q)\n"+
			"  active names for the recovered id: %v\n"+
			"  MEMORY.md =\n%s",
			byName.Memory.ID, first.Memory.ID, names, mustReadString(t, filepath.Join(store.Dir, indexFile)))
	}
}

// A failed publish, not only a killed one, can leave both hidden copies; the
// retry by id must repair that state through the same recovery rule.
func TestFailedPublishIsRecoveredByTheRetry(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	failRenames(t, func(path string) bool {
		return strings.HasPrefix(path, ".staged-") || strings.HasPrefix(path, ".retired-")
	})
	_, saveErr := store.SaveWithOptions(Memory{
		ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2",
	}, SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true})
	stopFailing(store.Dir)
	if saveErr == nil {
		t.Fatal("the injected rename failures did not fail the publish")
	}
	retired, _ := filepath.Glob(filepath.Join(store.Dir, ".retired-*"))
	staged, _ := filepath.Glob(filepath.Join(store.Dir, ".staged-*"))
	_, oldOK := mustStore(t, store).Read("alpha")
	if len(retired) == 0 || len(staged) == 0 || oldOK {
		t.Fatalf("the failed publish did not leave the interrupted-rename artifacts the probe needs: retired=%v staged=%v oldOK=%v (error %v)",
			retired, staged, oldOK, saveErr)
	}
	retried, err := store.SaveWithOptions(Memory{
		ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2",
	}, SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true})
	if err != nil {
		t.Fatalf("the retry after a failed publish was not repaired: %v", err)
	}
	names := namesInListForID(t, store, first.Memory.ID)
	if retried.Memory.ID != first.Memory.ID || retried.Memory.Revision != 2 || len(names) != 1 || names[0] != "beta" {
		t.Fatalf("the retry after a failed publish left the store inconsistent:\n"+
			"  retried id=%q revision=%d (want %q, 2)\n"+
			"  active names: %v\n"+
			"  MEMORY.md =\n%s",
			retried.Memory.ID, retried.Memory.Revision, first.Memory.ID, names,
			mustReadString(t, filepath.Join(store.Dir, indexFile)))
	}
}

// A retired copy left by a committed rename (its cleanup failed) has no staged
// partner, so it is not an interrupted publish and must not be revived.
func TestUnpairedRetiredCopyIsNotTreatedAsInterrupted(t *testing.T) {
	store := Store{Dir: testenv.TempDir(t)}
	first, err := store.SaveWithOptions(Memory{Name: "alpha", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	withFault(t, func(op, path string) error {
		if op == "remove" {
			return &os.PathError{Op: "remove", Path: path, Err: os.ErrPermission}
		}
		return nil
	})
	if _, err := store.SaveWithOptions(Memory{
		ID: first.Memory.ID, Name: "beta", Description: "renamed", Body: "v2",
	}, SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}); err != nil {
		t.Fatalf("the committed rename reported failure: %v", err)
	}
	stopFailing(store.Dir)
	if _, err := store.Archive("beta"); err != nil {
		t.Fatal(err)
	}
	retired, _ := filepath.Glob(filepath.Join(store.Dir, ".retired-*"))
	if len(retired) == 0 {
		t.Fatal("the probe needs the stale retired copy the committed rename failed to remove")
	}
	if _, err := store.SaveWithOptions(Memory{
		ID: first.Memory.ID, Name: "gamma", Description: "revived", Body: "v3",
	}, SaveOptions{}); err == nil {
		t.Fatalf("a stale retired copy with no staged partner was revived as an active fact")
	}
	if revived, ok, _ := store.Read(first.Memory.ID); ok {
		t.Fatalf("the archived id came back as %+v", revived)
	}
}

// A cross-scope move keeps its two hidden copies in different directories;
// recovery must pair them there and let the retried move complete.
func TestKillDuringCrossScopeMoveIsRecoveredByTheRetry(t *testing.T) {
	root := testenv.TempDir(t)
	store := Store{Dir: filepath.Join(root, "project"), GlobalDir: filepath.Join(root, "global")}
	if err := os.MkdirAll(store.GlobalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := store.SaveWithOptions(Memory{Name: "project/alpha.md", Description: "first", Body: "v1"}, SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	intent := Memory{ID: first.Memory.ID, Name: "beta", Scope: FactScopeGlobal, Description: "moved", Body: "v2"}
	opts := SaveOptions{ExpectedRevision: 1, RequireExpectedRevision: true}
	if killed := killDuring(t, isIndexPublish, func() { _, _ = store.SaveWithOptions(intent, opts) }); !killed {
		t.Fatal("the probe never reached the index publication, so the window was not exercised")
	}
	retried, err := store.SaveWithOptions(intent, opts)
	if err != nil {
		t.Fatalf("the retry after a killed cross-scope move was not repaired: %v", err)
	}
	names := namesInListForID(t, store, first.Memory.ID)
	if retried.Memory.ID != first.Memory.ID || retried.Memory.Scope != FactScopeGlobal ||
		len(names) != 1 || names[0] != "beta" {
		t.Fatalf("the recovered cross-scope move is inconsistent:\n"+
			"  retried id=%q scope=%s name=%q (want %q global beta)\n"+
			"  active names: %v\nproject MEMORY.md =\n%s\nglobal MEMORY.md =\n%s",
			retried.Memory.ID, retried.Memory.Scope, retried.Memory.Name, first.Memory.ID, names,
			mustReadString(t, filepath.Join(store.Dir, indexFile)),
			mustReadString(t, filepath.Join(store.GlobalDir, indexFile)))
	}
	if _, ok := mustStore(t, store).Read("project/alpha.md"); ok {
		t.Fatal("the moved fact stayed active under its old project name")
	}
}
