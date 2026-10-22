package boot

import (
	"context"
	"encoding/json"
	"strings"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/extension/sidecar"
)

type extensionDiscoveryTool struct {
	pluginID string
	clients  *sidecar.Manager
	registry *tool.Registry
}

func (t extensionDiscoveryTool) Name() string {
	name := strings.ReplaceAll(t.pluginID, ".", "_")
	return "connect_ext__" + name[:min(len(name), 42)] + "_" + agentcomposition.Digest(t.pluginID)[:8]
}
func (t extensionDiscoveryTool) Description() string {
	return "Connect extension " + t.pluginID + " and list its available tools."
}
func (t extensionDiscoveryTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}
func (t extensionDiscoveryTool) ExtensionPackageName() string { return t.pluginID }
func (t extensionDiscoveryTool) ReadOnly() bool               { return false }
func (t extensionDiscoveryTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	client, err := t.clients.EnsureReady(ctx, t.pluginID)
	if err != nil {
		return "", err
	}
	for _, decl := range client.DeclaredTools() {
		if client.ServesTool(decl.Name) {
			t.registry.Add(extensionTool{name: extensionToolName(t.pluginID, decl.Name), pluginID: t.pluginID, decl: decl, clients: t.clients})
		}
	}
	data, err := json.Marshal(client.DeclaredTools())
	return string(data), err
}
