package boot

import (
	"encoding/json"
	"fmt"
	"testing"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/state/agentpreset"
)

func TestLiveCompositionResourceReadRechecksServerGrant(t *testing.T) {
	home, root := compositionTestWorkspace(t, "openai")
	t.Setenv("REASONIX_HOME", home)
	server := newResourceMCPServer(t, true)
	configText := fmt.Sprintf("[tools]\nmcp_resources=true\n[[plugins]]\nname='res'\ntype='http'\nurl=%q\n", server.URL)
	writeFile(t, home, "config.toml", configText)
	warm, err := Build(t.Context(), Options{Home: home, WorkspaceRoot: root, resolvedShell: pinnedEffectShell()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := warm.ConnectMCPServer(warm.ConfiguredMCPServers()[0].Entry); err != nil {
		warm.Close()
		t.Fatal(err)
	}
	warm.Close()
	d := agentcomposition.Builtins()[0]
	d.ID = "user:resources"
	if _, err := agentpreset.New(home, root).Save(d, ""); err != nil {
		t.Fatal(err)
	}
	res, err := BuildRuntime(t.Context(), Options{Home: home, WorkspaceRoot: root, AgentComposition: d.ID, resolvedShell: pinnedEffectShell()})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Controller.Close()
	target, ok := res.Assembly.Registry.Get(plugin.ResourceReadToolName)
	if !ok {
		t.Fatal("cached resource capability is missing")
	}
	before := agentcomposition.Digest(res.Assembly.Registry.Schemas())
	args := json.RawMessage(`{"server":"res","uri":"doc://handbook"}`)
	if _, err := target.Execute(t.Context(), args); err != nil {
		t.Fatalf("authorized lazy resource read failed: %v", err)
	}
	writeFile(t, home, "config.toml", "[tools]\nmcp_resources=true\n")
	if _, err := target.Execute(t.Context(), args); err == nil {
		t.Fatal("removed server remained readable")
	}
	if len(server.readURIs()) != 1 {
		t.Fatal("removed server received another resource read")
	}
	if before != agentcomposition.Digest(res.Assembly.Registry.Schemas()) {
		t.Fatal("resource connection changed provider-visible schema")
	}
}
