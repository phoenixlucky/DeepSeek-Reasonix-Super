package boot

import (
	"crypto/rand"
	"reasonix/internal/ext/mcplaunch"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/ext/pluginspec"
	"time"
)

// wireMCP registers the MCP servers and LSP tools and chains their shutdown
// into the controller's cleanup. A caller-supplied shared host lets
// controllers for one workspace reuse running MCP processes; its owner closes it.
func (b *builder) wireMCP() error {
	opts, cfg, root, t := b.opts, b.cfg, b.root, &b.tools
	t.host = opts.SharedHost
	if t.host == nil {
		t.host = plugin.NewHost()
	}
	// A lazy server connects in the background; without a status sink every
	// status view keeps showing what it saw at boot.
	t.host.SetStatusSink(opts.Sink)
	t.specOptions = pluginspec.Options{
		DefaultStartupTimeout: time.Duration(cfg.MCPStartupTimeoutSeconds()) * time.Second,
		DefaultCallTimeout:    time.Duration(cfg.MCPCallTimeoutSeconds()) * time.Second,
		LaunchManager:         mcplaunch.ForWorkspace(b.roots.Home(), root),
		ConfigSource:          "workspace_config",
		StateHome:             b.roots.Home(),
		WriterRoots:           t.env.writeRoots,
		WritableRoots:         t.env.writeRoots,
		ForbidReadRoots:       t.env.forbidReadRoots,
		Network:               t.env.network,
		PackageOwners:         pluginspec.PackageOwners(cfg),
		OAuthHTTPClient:       b.balanceClient,
	}
	if opts.Composition != nil {
		t.specOptions.ProcessScope = rand.Text()
	}
	t.mcp = resolveMCPSpecs(opts, cfg, root, t.specOptions)
	b.cleanup = t.host.Close
	if opts.SharedHost != nil {
		b.cleanup = func() {}
	}
	if err := b.isolateCompositionMCPPackages(); err != nil {
		return err
	}
	reportProjectMCPAwaitingApproval(b.sink, cfg, root)
	if opts.Composition.Live() {
		t.configSpecs, t.mcpSchemaKnown = registerLiveCompositionMCP(b.ctx, t.host, t.reg, t.mcp)
	} else {
		t.configSpecs, t.mcpSchemaKnown = registerMCPTools(b.ctx, t.host, t.reg, t.mcp, b.sink)
	}
	registerResourceTool(t.host, t.reg, cfg, t.mcp, opts.Composition.Live())
	// The LSP manager is session-scoped: its servers stop with the controller.
	if t.lsp = registerLSP(t.reg, cfg, root); t.lsp != nil {
		prev, mgr := b.cleanup, t.lsp
		b.cleanup = func() { prev(); mgr.Close() }
	}
	if proxy := t.env.egress; proxy != nil {
		prev := b.cleanup
		b.cleanup = func() { prev(); _ = proxy.Close() }
	}
	b.cleanup = closeAfter(b.cleanup, b.trace)
	return nil
}
