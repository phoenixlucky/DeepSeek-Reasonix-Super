package control

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/state/memory"
)

// The recall-freshness delivery/change matrix: one subtest per (delivery, change
// time, change shape) cell. "old" is allowed only while the fact was never
// delivered; a change landing after a delivery must be what the next recall
// carries. See mxShapes, mxTimelines and mxCheck for the axes and the tokens.
const (
	mxQuery = "staging deploy target"
	mxQuiet = "completely unrelated words about lunch"
	mxOld   = "old body marker"
	mxNew   = "rewritten body marker with a much longer tail"
	mxSame  = "new body marker" // exactly as long as mxOld
	mxRepl  = "replacement body marker from a different fact"
	mxName  = "deploy-target"
)

type mxShape struct {
	key   string
	label string
	// want is the body the fact's file reads after the change; "" when the fact
	// must be gone instead.
	want string
}

// Change time: i before the first delivery, ii after a delivery, iii after a
// no-hit turn, iv between two deliveries. Shape: alpha rewrite, beta same
// length with mtime restored, gamma delete, delta scope move, epsilon new fact.
var mxShapes = []mxShape{
	{"alpha", "content rewritten, different length", mxNew},
	{"beta", "same length, modification time restored", mxSame},
	{"gamma", "file deleted", ""},
	{"delta", "scope changed so the canonical path moves", mxNew},
	{"epsilon", "different fact (different id) at the same path", mxRepl},
}

// mxTimeline is one (delivery, change time) script. steps runs over Q (recall),
// U (no-hit recall), M (the change) and S (a second save); want holds one token
// per Q/U, in order: old (the delivered body, allowed only before delivery), new
// (the change), gone (no body at all), quiet (the no-hit turn recalls nothing).
type mxTimeline struct {
	delivery string
	when     string
	noted    bool // the fact is created through SaveMemory, so its first turn carries a note
	steps    string
	want     string
}

var mxTimelines = []mxTimeline{
	// (a) delivered as a recall hit only.
	{"a", "i", false, "M Q Q", "old new"},
	{"a", "ii", false, "Q M Q", "old new"},
	{"a", "iii", false, "Q U M Q", "old quiet new"},
	{"a", "iv", false, "Q Q M Q", "old old new"},

	// (b) delivered only by the save note.
	{"b", "i", true, "M Q Q", "old new"},
	{"b", "ii", true, "Q M Q", "old new"},
	{"b", "iii", true, "Q U M Q", "old quiet new"},
	{"b", "iv", true, "Q S Q M Q", "old old new"},

	// (c) note-carried first, then also a recall hit.
	{"c", "i", true, "M Q Q", "old new"},
	{"c", "ii", true, "Q Q M Q", "old old new"},
	{"c", "iii", true, "Q Q U M Q", "old old quiet new"},
	{"c", "iv", true, "Q Q Q M Q", "old old old new"},

	// (d) two recalls in a row, the second returning no hits.
	{"d", "i", false, "M Q U Q", "old quiet new"},
	{"d", "ii", false, "Q M U Q", "old quiet new"},
	{"d", "iii", false, "Q U M Q", "old quiet new"},
	{"d", "iv", false, "Q U Q M Q", "old quiet old new"},

	// (e) a recall with hits after a no-hit recall.
	{"e", "i", false, "M U Q Q", "quiet old new"},
	{"e", "ii", false, "U Q M Q", "quiet old new"},
	{"e", "iii", false, "U M Q Q", "quiet old new"},
	{"e", "iv", false, "U Q Q M Q", "quiet old old new"},
}

func mxFact() memory.Memory {
	return memory.Memory{
		Name:        mxName,
		Description: "where the staging deploy goes",
		Keywords:    "staging deploy target",
		Type:        memory.TypeProject,
		Scope:       memory.FactScopeProject,
		Body:        mxOld,
	}
}

type mxEnv struct {
	c     *Controller
	path  string
	store memory.Store
	fact  memory.Memory
}

// mxSession builds the session for a cell. A noted fact is written through the
// controller (so a turn-tail note is what first carries it); an un-noted fact is
// seeded into the same store before the session loads, so recall is its first
// delivery.
func mxSession(t *testing.T, noted bool) *mxEnv {
	t.Helper()
	root := testenv.TempDir(t)
	cwd := filepath.Join(root, "project")
	userDir := filepath.Join(root, "user")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	store := memory.StoreFor(userDir, cwd)
	var (
		c    *Controller
		path string
	)
	if noted {
		c = New(Options{Memory: memory.Load(memory.Options{CWD: cwd, UserDir: userDir})})
		written, err := c.SaveMemory(mxFact())
		if err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
		path = written
	} else {
		written, err := store.Save(mxFact())
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		path = written
		c = New(Options{Memory: memory.Load(memory.Options{CWD: cwd, UserDir: userDir})})
	}
	fact, ok, err := store.Read(mxName)
	if err != nil || !ok {
		t.Fatalf("the seeded fact is not readable back: ok=%v err=%v", ok, err)
	}
	return &mxEnv{c: c, path: path, store: store, fact: fact}
}

// mxMutate applies the external change. Each shape proves the change it claims
// before returning, so a cell cannot pass because the change never landed.
func mxMutate(t *testing.T, env *mxEnv, shape mxShape) {
	t.Helper()
	switch shape.key {
	case "alpha":
		rewriteOnDisk(t, env.path, mxOld, mxNew)
	case "beta":
		before, err := os.Stat(env.path)
		if err != nil {
			t.Fatal(err)
		}
		rewriteOnDisk(t, env.path, mxOld, mxSame)
		if err := os.Chtimes(env.path, before.ModTime(), before.ModTime()); err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(env.path)
		if err != nil {
			t.Fatal(err)
		}
		if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			t.Fatalf("shape beta must be invisible to a stat: size %d→%d, mtime %v→%v",
				before.Size(), after.Size(), before.ModTime(), after.ModTime())
		}
	case "gamma":
		if err := os.Remove(env.path); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(env.path); !os.IsNotExist(err) {
			t.Fatalf("shape gamma must leave no file at %s: %v", env.path, err)
		}
	case "delta":
		raw, err := os.ReadFile(env.path)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Replace(string(raw), mxOld, mxNew, 1)
		if text == string(raw) {
			t.Fatalf("shape delta found no body to rewrite:\n%s", raw)
		}
		text = strings.Replace(text, "scope: project", "scope: global", 1)
		if !strings.Contains(text, "scope: global") {
			t.Fatalf("shape delta did not move the fact's scope:\n%s", raw)
		}
		moved := filepath.Join(env.store.GlobalDir, mxName+".md")
		if err := os.MkdirAll(env.store.GlobalDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(moved, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(env.path); err != nil {
			t.Fatal(err)
		}
		if moved == env.path {
			t.Fatalf("shape delta did not move the canonical path (%s)", moved)
		}
	case "epsilon":
		raw, err := os.ReadFile(env.path)
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Replace(string(raw), "id: "+env.fact.ID, "id: "+env.fact.ID+"-other", 1)
		if text == string(raw) {
			t.Fatalf("shape epsilon found no id line to replace (%q):\n%s", env.fact.ID, raw)
		}
		text = strings.Replace(text, mxOld, mxRepl, 1)
		if err := os.WriteFile(env.path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		replaced, ok, err := env.store.Read(env.fact.ID)
		if err != nil {
			t.Fatal(err)
		}
		if ok && replaced.ID == env.fact.ID {
			t.Fatal("shape epsilon left the same fact at the path")
		}
	default:
		t.Fatalf("unknown shape %q", shape.key)
	}
}

// mxCheck asserts one turn's token. It reports the cell name and the turn
// number, so a red run names the matrix cell and which turn broke.
func mxCheck(t *testing.T, cell string, turn int, token, out string, shape mxShape) {
	t.Helper()
	hasOld := strings.Contains(out, mxOld)
	hasNew := shape.want != "" && strings.Contains(out, shape.want)
	fail := func(why string) {
		t.Fatalf("%s: turn %d (%s) %s\n--- turn ---\n%s", cell, turn, token, why, out)
	}
	switch token {
	case "old":
		if !hasOld {
			fail(fmt.Sprintf("does not carry the delivered body %q, so the cell answers nothing", mxOld))
		}
		if hasNew {
			fail("already carries the change, which the promised turn order does not allow")
		}
	case "new":
		if !hasNew {
			fail(fmt.Sprintf("does not carry the change %q", shape.want))
		}
		if hasOld {
			fail(fmt.Sprintf("still carries the stale delivered body %q", mxOld))
		}
	case "gone":
		if hasOld {
			fail("still carries a body for a fact whose file is gone")
		}
	case "quiet":
		if hasOld || hasNew {
			fail("was supposed to recall nothing")
		}
	default:
		t.Fatalf("%s: unknown want token %q", cell, token)
	}
}

// The reload-count companion to the matrix: a no-hit recall that detects a change
// already reloads the snapshot, and the settled snapshot is what the next turn
// serves — so a fact that has not changed again must not reload the store again.
// Body assertions cannot see this, and the pre-branch code cleared the re-check
// set on reload, so its stamp described the bytes from before the reload.
func TestRecallReloadsTheSnapshotOncePerChange(t *testing.T) {
	// assertSettled runs a no-hit recall twice, holding the snapshot pointer
	// between them: the first settles the change already applied on disk, the
	// second must find nothing left to settle.
	assertSettled := func(t *testing.T, c *Controller) {
		t.Helper()
		if out := c.Compose(mxQuiet); strings.Contains(out, mxOld) || strings.Contains(out, mxNew) {
			t.Fatalf("the quiet turn was supposed to recall nothing:\n%s", out)
		}
		settled := c.Memory()
		if settled == nil {
			t.Fatal("the session has no memory snapshot")
		}
		c.Compose(mxQuiet)
		if again := c.Memory(); again != settled {
			t.Fatalf("a settled change reloaded the whole store again on the next turn:\n"+
				"the freshness check still holds the pre-reload stamp, so every recall re-detects the same change\n"+
				"held before=%p after=%p", settled, again)
		}
	}

	t.Run("rewritten", func(t *testing.T) {
		c, path := mxDelivered(t)
		rewriteOnDisk(t, path, mxOld, mxNew)
		assertSettled(t, c)
	})

	t.Run("deleted", func(t *testing.T) {
		c, path := mxDelivered(t)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		assertSettled(t, c)
	})

	t.Run("settled by an unrelated write", func(t *testing.T) {
		c, path := mxDelivered(t)
		rewriteOnDisk(t, path, mxOld, mxNew)
		// A write reloads the snapshot too; the change is settled there just as
		// it is when the freshness check is what asks for the reload.
		if _, err := c.SaveMemory(memory.Memory{
			Name: "unrelated", Description: "an unrelated fact", Keywords: "unrelated",
			Type: memory.TypeProject, Body: "unrelated body marker",
		}); err != nil {
			t.Fatalf("SaveMemory: %v", err)
		}
		c.Compose("hello") // spend the write's turn-tail note
		assertSettled(t, c)
	})
}

// mxDelivered returns a session whose target fact has been delivered as a recall
// hit, so the freshness check is watching it.
func mxDelivered(t *testing.T) (*Controller, string) {
	t.Helper()
	c, path := externalEditSession(t)
	if first := c.Compose(mxQuery); !strings.Contains(first, mxOld) {
		t.Fatalf("the probe proves nothing unless the turn delivers the fact:\n%s", first)
	}
	return c, path
}

func TestMemoryDeliveryChangeMatrix(t *testing.T) {
	for _, timeline := range mxTimelines {
		for _, shape := range mxShapes {
			cell := fmt.Sprintf("delivery=%s/when=%s/shape=%s", timeline.delivery, timeline.when, shape.key)
			t.Run(cell, func(t *testing.T) {
				env := mxSession(t, timeline.noted)
				want := strings.Fields(timeline.want)
				for i, token := range want {
					if token == "new" && shape.want == "" {
						want[i] = "gone"
					}
				}
				steps := strings.Fields(timeline.steps)
				turn := 0
				for _, step := range steps {
					switch step {
					case "M":
						mxMutate(t, env, shape)
					case "S":
						if _, err := env.c.SaveMemory(mxFact()); err != nil {
							t.Fatalf("%s: second SaveMemory: %v", cell, err)
						}
					case "Q", "U":
						text := mxQuery
						if step == "U" {
							text = mxQuiet
						}
						out := env.c.Compose(text)
						if turn >= len(want) {
							t.Fatalf("%s: step %q has no expected token", cell, step)
						}
						mxCheck(t, cell, turn+1, want[turn], out, shape)
						turn++
					default:
						t.Fatalf("%s: unknown step %q", cell, step)
					}
				}
				if turn != len(want) {
					t.Fatalf("%s: %d compose turns ran, %d expected", cell, turn, len(want))
				}
			})
		}
	}
}
