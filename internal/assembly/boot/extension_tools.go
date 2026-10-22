package boot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/extension/protocol"
	"reasonix/internal/ext/extension/sidecar"
	"reasonix/internal/ext/pluginpkg"
)

// extensionToolTimeout bounds one call; a tool doing longer work should
// report progress through its own surfaces and return.
const extensionToolTimeout = 5 * time.Minute

// extensionTool is a tool a plugin runtime serves. Like an MCP tool it is not
// in the provider-visible list: the model finds it through the capability
// catalog, so installing or reloading a plugin never moves the cached prefix.
type extensionTool struct {
	name     string
	pluginID string
	decl     pluginpkg.RuntimeTool
	clients  *sidecar.Manager
}

func (t extensionTool) Name() string                 { return t.name }
func (t extensionTool) Description() string          { return t.decl.Description }
func (t extensionTool) Schema() json.RawMessage      { return t.decl.InputSchema }
func (t extensionTool) ReadOnly() bool               { return t.decl.ReadOnly }
func (t extensionTool) ExtensionPackageName() string { return t.pluginID }

func (t extensionTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var res protocol.ToolCallResult
	err := t.clients.WithReady(ctx, t.pluginID, func(callCtx context.Context, client *sidecar.Client) error {
		if !client.ServesTool(t.decl.Name) {
			return fmt.Errorf("extension %s no longer serves tool %s", t.pluginID, t.decl.Name)
		}
		for _, current := range client.DeclaredTools() {
			if current.Name == t.decl.Name && agentcomposition.Digest(current) != agentcomposition.Digest(t.decl) {
				return agentcomposition.Unavailable("extension:"+t.pluginID, "Tool contract changed before dispatch; refresh the capability directory and retry.")
			}
		}
		var callErr error
		res, callErr = client.CallTool(callCtx, t.decl.Name, args, extensionToolTimeout)
		return callErr
	})
	if err != nil {
		return "", fmt.Errorf("the %s extension could not run %s: %w", t.pluginID, t.decl.Name, err)
	}
	if res.IsError {
		return "", errors.New(res.Content)
	}
	return res.Content, nil
}

// extensionToolName qualifies a runtime's tool with its plugin, so two plugins
// can each serve a "search" and neither can take a built-in's name.
func extensionToolName(pluginID, toolName string) string {
	return tool.ExtensionNamePrefix + strings.NewReplacer(".", "_").Replace(pluginID) + "__" + toolName
}

// addExtensionTools registers the tools each running runtime declared in its
// manifest and activated at its handshake. A runtime that failed to start
// contributes none, so the catalog never lists a tool nothing can run.
func addExtensionTools(reg *tool.Registry, mgr *sidecar.Manager, warn func(string)) {
	if mgr == nil {
		return
	}
	if mgr.OnDemand() {
		reg.RemovePrefix(tool.ExtensionNamePrefix)
		reg.RemovePrefix("connect_ext__")
	}
	for _, p := range mgr.DeferredPackages() {
		for _, decl := range p.Package.Manifest.Runtime.Tools {
			name := extensionToolName(p.Installed.Name, decl.Name)
			if len(name) <= 64 {
				reg.Add(extensionTool{name: name, pluginID: p.Installed.Name, decl: decl, clients: mgr})
			}
		}
		if p.Package.Manifest.Runtime.DynamicTools {
			reg.Add(extensionDiscoveryTool{pluginID: p.Installed.Name, clients: mgr, registry: reg})
		}
	}
	for _, client := range mgr.ToolClients() {
		for _, decl := range client.DeclaredTools() {
			if !client.ServesTool(decl.Name) {
				continue
			}
			name := extensionToolName(client.PluginID(), decl.Name)
			if len(name) > 64 {
				warn(fmt.Sprintf("extension %s: tool %s is not offered: %s is over the 64 characters a model tool name may have", client.PluginID(), decl.Name, name))
				continue
			}
			reg.Add(extensionTool{name: name, pluginID: client.PluginID(), decl: decl, clients: mgr})
		}
	}
}
