package boot

import (
	"context"
	"os"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
)

// The detailed level reaches the provider only as a turn-tail block: the stable
// system prompt and the tool table stay byte-identical, brief adds nothing, a
// project cannot turn it on, and the choice persists in the user config.
func TestEffectTurnNarrationRidesTheTurnTail(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	rec := &effectRecordingProvider{}
	provider.Register("boot-turn-narration", func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
turn_narration = "detailed"

[[providers]]
name = "test-model"
kind = "boot-turn-narration"
model = "x"
`)
	approveWorkspace(t, dir)

	ctrl, err := Build(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	if err := ctrl.Run(context.Background(), "first question"); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if err := ctrl.SaveTurnNarrationSettings("detailed"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := ctrl.Run(context.Background(), "second question"); err != nil {
		t.Fatalf("second turn: %v", err)
	}
	raw, err := os.ReadFile(config.UserConfigPath())
	if err != nil {
		t.Fatalf("read stored config: %v", err)
	}
	if !strings.Contains(string(raw), `turn_narration = "detailed"`) {
		t.Errorf("the detailed choice was not stored:\n%s", raw)
	}
	if err := ctrl.SaveTurnNarrationSettings("brief"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := ctrl.Run(context.Background(), "third question"); err != nil {
		t.Fatalf("third turn: %v", err)
	}

	reqs := agentRequests(rec.requests())
	if len(reqs) != 3 {
		t.Fatalf("expected three requests, got %d", len(reqs))
	}
	if strings.Contains(lastUserOf(reqs[0]), "<turn-narration>") {
		t.Error("a project's turn_narration turned the instruction on; only the user's own setting may")
	}
	if !strings.Contains(lastUserOf(reqs[1]), "<turn-narration>") {
		t.Errorf("the detailed level never reached the next turn: %q", lastUserOf(reqs[1]))
	}
	if strings.Contains(lastUserOf(reqs[2]), "<turn-narration>") {
		t.Error("going back to brief left the instruction on")
	}
	if systemOf(reqs[0]) != systemOf(reqs[1]) || systemOf(reqs[1]) != systemOf(reqs[2]) {
		t.Error("the narration level moved the cache-stable system prefix")
	}
	raw, err = os.ReadFile(config.UserConfigPath())
	if err != nil {
		t.Fatalf("read stored config: %v", err)
	}
	if strings.Contains(string(raw), `turn_narration = "detailed"`) {
		t.Errorf("going back to brief left the stored choice on:\n%s", raw)
	}
}
