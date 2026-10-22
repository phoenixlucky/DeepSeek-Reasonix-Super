package serve

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

func TestMovingABoundConversationRefusesBeforeChangingEitherProject(t *testing.T) {
	_, srv, a, b := twoProjects(t)
	d := agentcomposition.Builtins()[1]
	binding := &agentcomposition.Binding{Version: agentcomposition.Version{Definition: d, Revision: agentcomposition.Revision(d)}, Resolved: d, Workspace: a}
	binding.Seal()
	session := sessionstore.NewSession("bound prompt")
	session.BindComposition(binding)
	session.Add(provider.Message{Role: provider.RoleUser, Content: "keep the exact composition"})
	path := filepath.Join(SessionDirFor(a), "20261018-000000-bound.jsonl")
	if err := session.SaveSnapshot(path); err != nil {
		t.Fatal(err)
	}
	status, out := moveTo(t, srv, path, b)
	if status != http.StatusConflict || out["code"] != "agent_composition.workspace_bound" {
		t.Fatalf("move did not refuse the binding change: %d %v", status, out)
	}
	if _, err := os.Stat(sessionstore.SessionEventLogPath(path)); err != nil {
		t.Fatalf("source disappeared: %v", err)
	}
	if got := namesIn(t, srv, b); len(got) != 0 {
		t.Fatalf("target received an unbound copy: %v", got)
	}
}
