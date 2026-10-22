package serve

import (
	"context"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
	"reasonix/internal/state/storage"
)

// A session path the window remembered before the state root moved still names
// the same conversation: it has to open, not answer session.unknown.
func TestASessionPathFromBeforeAStateRootMoveStillOpens(t *testing.T) {
	root := testenv.TempDir(t)
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	h := NewHub(HubOptions{ProviderResolver: homeResolver()})
	defer h.Shutdown()
	oldPath := filepath.Join(SessionDirFor(root), "20261001-120000.jsonl")
	s := sessionstore.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "before the move"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "an answer"})
	if err := s.SaveSnapshot(oldPath); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(testenv.TempDir(t), "moved-state")
	plan := storage.PlanMove(context.Background(), config.RootState, target)
	if !plan.OK() {
		t.Fatalf("plan refused: %+v", plan.Refusals)
	}
	if err := storage.Move(context.Background(), plan, func(storage.Progress) {}); err != nil {
		t.Fatalf("move: %v", err)
	}

	rt, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: oldPath})
	if err != nil {
		t.Fatalf("opening the pre-move path: %v", err)
	}
	defer h.Close(rt.ID)
	if got, want := filepath.Base(rt.Server.Controller().SessionPath()), filepath.Base(oldPath); got != want {
		t.Fatalf("opened %s, want the moved copy of %s", got, want)
	}
}

func TestAPathThatIsNotTheMovedSessionIsStillRefused(t *testing.T) {
	root := testenv.TempDir(t)
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	h := NewHub(HubOptions{ProviderResolver: homeResolver()})
	defer h.Shutdown()
	dir := SessionDirFor(root)
	s := sessionstore.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "kept"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "an answer"})
	if err := s.SaveSnapshot(filepath.Join(dir, "20261001-120000.jsonl")); err != nil {
		t.Fatal(err)
	}
	elsewhere := testenv.TempDir(t)
	cases := map[string]string{
		"dot-dot":        elsewhere + string(filepath.Separator) + "x" + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(dir) + string(filepath.Separator) + "20261001-120000.jsonl",
		"another folder": filepath.Join(elsewhere, "gone", "20261001-120000.jsonl"),
		"other name":     filepath.Join(elsewhere, filepath.Base(dir), "20261001-999999.jsonl"),
	}
	for name, p := range cases {
		_, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: p})
		if err == nil {
			t.Errorf("%s: %s opened", name, p)
		}
	}
}

// A remembered path from another project names that project's conversation, not
// this one's: a file of the same name in this project's directory must not stand
// in for it.
func TestAPathFromAnotherProjectIsNotResolvedIntoThisOne(t *testing.T) {
	root := testenv.TempDir(t)
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	h := NewHub(HubOptions{ProviderResolver: homeResolver()})
	defer h.Shutdown()
	dir := SessionDirFor(root)
	const name = "20261001-120000.jsonl"
	s := sessionstore.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "this project's talk"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "an answer"})
	if err := s.SaveSnapshot(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(filepath.Dir(dir)), "some-other-project", "sessions", name)
	if rt, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: other}); err == nil {
		h.Close(rt.ID)
		t.Fatalf("a path under another project's session folder opened %s", rt.Server.Controller().SessionPath())
	}
}
