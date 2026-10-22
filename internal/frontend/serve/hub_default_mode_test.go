package serve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

// Which opens are a new conversation, and so take the user's default mode: a
// pane opened with no session to continue is, and nothing that names a saved
// conversation (a branch, an import, a restored tab, a fork) is. A path that
// does not exist is refused rather than quietly turned into a new conversation.
func TestHubOpensTakeTheDefaultModeOnlyWhenTheyAreNewConversations(t *testing.T) {
	root := testenv.TempDir(t)
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	f, err := os.OpenFile(config.UserConfigPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n[agent]\ndefault_mode = \"plan\"\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	h := NewHub(HubOptions{ProviderResolver: homeResolver()})
	defer h.Shutdown()

	saved := filepath.Join(SessionDirFor(root), "20261001-120000.jsonl")
	s := sessionstore.NewSession("system")
	s.Add(provider.Message{Role: provider.RoleUser, Content: "an earlier talk"})
	s.Add(provider.Message{Role: provider.RoleAssistant, Content: "an answer"})
	if err := s.SaveSnapshot(saved); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		path     string
		wantPlan bool
		refused  bool
	}{
		"a new tab with no session": {path: "", wantPlan: true},
		"a blank session path":      {path: "  ", wantPlan: true},
		"a saved conversation, as a branch, an import or a restored tab opens it": {path: saved, wantPlan: false},
		"a path that no longer exists is refused, not started fresh":              {path: filepath.Join(SessionDirFor(root), "20260101-000000.jsonl"), refused: true},
	} {
		t.Run(name, func(t *testing.T) {
			rt, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: tc.path})
			if tc.refused {
				if err == nil {
					h.Close(rt.ID)
					t.Fatal("a missing session path opened a pane")
				}
				return
			}
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer h.Close(rt.ID)
			if got := rt.Server.Controller().PlanMode(); got != tc.wantPlan {
				t.Fatalf("plan mode = %v, want %v", got, tc.wantPlan)
			}
		})
	}
}
