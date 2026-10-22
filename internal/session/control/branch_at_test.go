package control

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

func orderedTurns(t *testing.T, c *Controller) []int {
	t.Helper()
	c.checkpoints.mu.Lock()
	defer c.checkpoints.mu.Unlock()
	turns := make([]int, 0, len(c.checkpoints.bound))
	for turn := range c.checkpoints.bound {
		turns = append(turns, turn)
	}
	slices.Sort(turns)
	return turns
}

func savedMessages(t *testing.T, path string) int {
	t.Helper()
	loaded, err := sessionstore.LoadSession(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	return len(loaded.Messages)
}

func TestBranchAtKeepsTheChosenTurnAndNothingAfterIt(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	turns := orderedTurns(t, c)
	if len(turns) != 2 {
		t.Fatalf("turns = %v, want two", turns)
	}
	parent := c.SessionPath()
	secondStart, _ := c.checkpoints.boundary(turns[1])
	total := len(ag.Session().Messages)

	path, err := c.BranchAt(turns[0], "first answer, another way")
	if err != nil {
		t.Fatalf("BranchAt: %v", err)
	}
	if path == "" || path == parent {
		t.Fatalf("branch path = %q, want a new session beside %q", path, parent)
	}
	if got := savedMessages(t, path); got != secondStart {
		t.Fatalf("branch holds %d messages, want %d: the first turn whole and none of the second", got, secondStart)
	}
	if c.SessionPath() != parent || len(ag.Session().Messages) != total {
		t.Fatalf("the source conversation moved: path %q messages %d, want %q and %d", c.SessionPath(), len(ag.Session().Messages), parent, total)
	}
	meta, ok, err := sessionstore.LoadBranchMeta(path)
	if err != nil || !ok {
		t.Fatalf("branch meta ok=%v err=%v", ok, err)
	}
	if meta.ParentID != sessionstore.BranchID(parent) || meta.CustomTitle != "first answer, another way" {
		t.Fatalf("branch meta = %+v, want the source as parent and the given title", meta)
	}
}

func TestBranchAtTheLastTurnTakesTheWholeConversation(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	turns := orderedTurns(t, c)
	path, err := c.BranchAt(turns[len(turns)-1], "")
	if err != nil {
		t.Fatalf("BranchAt: %v", err)
	}
	if got := savedMessages(t, path); got != len(ag.Session().Messages) {
		t.Fatalf("branch holds %d messages, want the whole conversation (%d)", got, len(ag.Session().Messages))
	}
}

func TestBranchAtAnUnknownTurnIsRefusedWithAnIdentity(t *testing.T) {
	c, _, _ := runTwoTurns(t)
	if _, err := c.BranchAt(99, "x"); !errors.Is(err, ErrBranchUnavailable) {
		t.Fatalf("BranchAt(99) = %v, want ErrBranchUnavailable", err)
	}
}

func TestABranchIsListedBesideItsSourceUnderItsOwnTitle(t *testing.T) {
	c, _, _ := runTwoTurns(t)
	if err := c.Snapshot(); err != nil {
		t.Fatal(err)
	}
	turns := orderedTurns(t, c)
	path, err := c.BranchAt(turns[0], "the other way")
	if err != nil {
		t.Fatalf("BranchAt: %v", err)
	}
	listed, err := sessionstore.ListSessions(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	var source, branch *sessionstore.SessionInfo
	for i := range listed {
		switch listed[i].Path {
		case path:
			branch = &listed[i]
		case c.SessionPath():
			source = &listed[i]
		}
	}
	if source == nil || branch == nil {
		t.Fatalf("listing holds %d sessions; source found=%v branch found=%v", len(listed), source != nil, branch != nil)
	}
	if branch.Recovered || branch.CustomTitle != "the other way" || source.CustomTitle == branch.CustomTitle {
		t.Fatalf("branch = %+v, want an ordinary session with its own title (source %+v)", branch, source)
	}
}

func startLaterTurn(t *testing.T, c *Controller, ag interface {
	Session() *sessionstore.Session
}) {
	t.Helper()
	if err := c.beginCheckpoint(context.Background(), "third prompt"); err != nil {
		t.Fatal(err)
	}
	ag.Session().Add(provider.Message{Role: provider.RoleUser, Content: "third prompt", RawContent: "third prompt"})
	c.mu.Lock()
	c.gate.begin(func() {})
	c.mu.Unlock()
}

func TestBranchAtClosedTurnWhileALaterTurnRuns(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	turns := orderedTurns(t, c)
	secondStart, _ := c.checkpoints.boundary(turns[1])
	startLaterTurn(t, c, ag)
	total := len(ag.Session().Messages)

	path, err := c.BranchAt(turns[0], "")
	if err != nil {
		t.Fatalf("BranchAt a closed turn mid-run: %v", err)
	}
	if got := savedMessages(t, path); got != secondStart {
		t.Fatalf("branch holds %d messages, want %d: the first turn only", got, secondStart)
	}
	if _, err := c.BranchAt(turns[1], ""); err != nil {
		t.Fatalf("the latest closed turn must branch too: %v", err)
	}
	if len(ag.Session().Messages) != total {
		t.Fatalf("the running conversation changed: %d messages, want %d", len(ag.Session().Messages), total)
	}
	if _, err := c.BranchAt(turns[1]+1, ""); !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("the running turn itself: err = %v, want ErrTurnRunning", err)
	}
}

func TestBranchAtClosedTurnRefusesABoundaryThatNamesAnotherMessage(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	turns := orderedTurns(t, c)
	startLaterTurn(t, c, ag)
	at, _ := c.checkpoints.boundary(turns[1] + 1)

	msgs := ag.Session().Snapshot()
	msgs[at] = provider.Message{Role: provider.RoleUser, Content: "someone else's prompt", RawContent: "someone else's prompt"}
	ag.Session().Rewrite(msgs, "")

	if _, err := c.BranchAt(turns[1], ""); !errors.Is(err, ErrBranchStale) || !errors.Is(err, ErrBranchUnavailable) {
		t.Fatalf("a boundary now holding another prompt: err = %v, want ErrBranchStale", err)
	}

	shifted := append([]provider.Message{{Role: provider.RoleUser, Content: "inserted", RawContent: "inserted"}}, ag.Session().Snapshot()...)
	ag.Session().Rewrite(shifted, "")
	if _, err := c.BranchAt(turns[0], ""); !errors.Is(err, ErrBranchStale) {
		t.Fatalf("a boundary shifted by an earlier rewrite: err = %v, want ErrBranchStale", err)
	}
}
