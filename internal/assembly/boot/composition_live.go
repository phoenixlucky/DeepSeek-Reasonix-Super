package boot

import (
	"os"
	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/ext/pluginpkg"
)

func newLiveCompositionBinding(v agentcomposition.Version, root string) *agentcomposition.Binding {
	return &agentcomposition.Binding{
		FormatVersion: agentcomposition.LiveBindingVersion, ResourceMode: agentcomposition.LiveResources,
		Version: v, Resolved: agentcomposition.Clone(v.Definition), Workspace: root, Resources: []agentcomposition.Resource{},
	}
}

func (b *builder) prepareLiveComposition() error {
	binding := b.opts.Composition
	entries := []config.PluginEntry{}
	for _, entry := range b.cfg.Plugins {
		if !binding.Resolved.MCP.Allows(entry.Name) {
			continue
		}
		if owner, owned := b.cfg.PluginPackageOwner(entry.Name); owned && !binding.Resolved.Extensions.Allows(owner) {
			continue
		}
		for _, m := range binding.Resolved.MCP.Members {
			if m.ID == entry.Name {
				var err error
				entry, err = applyCompositionMCPParameters(entry, m.Parameters)
				if err != nil {
					return err
				}
			}
		}
		entries = append(entries, entry)
	}
	b.cfg.Plugins = entries
	extra := []plugin.Spec{}
	for _, spec := range b.opts.ExtraPlugins {
		if binding.Resolved.MCP.Allows(spec.Name) && (spec.Package == "" || binding.Resolved.Extensions.Allows(spec.Package)) {
			entry := compositionMCPEntry(spec)
			for _, m := range binding.Resolved.MCP.Members {
				if m.ID == spec.Name {
					var err error
					entry, err = applyCompositionMCPParameters(entry, m.Parameters)
					if err != nil {
						return err
					}
				}
			}
			_, resolved := splitCompositionMCP([]config.PluginEntry{entry}, []plugin.Spec{spec}, b.root)
			spec = resolved[0]
			extra = append(extra, spec)
		}
	}
	b.opts.ExtraPlugins = extra
	return b.resolveLiveHookSelection()
}

func liveCompositionPackages(packages []pluginpkg.InstalledPackage, binding *agentcomposition.Binding) ([]pluginpkg.InstalledPackage, error) {
	out := []pluginpkg.InstalledPackage{}
	for _, p := range packages {
		if !binding.Resolved.Extensions.Allows(p.Installed.Name) {
			continue
		}
		if p.Package.Manifest.Runtime == nil {
			continue
		}
		rt := *p.Package.Manifest.Runtime
		rt.TightenOnly = true
		p.Package.Manifest.Runtime = &rt
		for _, m := range binding.Resolved.Extensions.Members {
			if m.ID != p.Installed.Name || p.Package.Manifest.Runtime == nil {
				continue
			}
			params, err := parameters(m.Parameters)
			if err != nil {
				return nil, err
			}
			rt := *p.Package.Manifest.Runtime
			if params.Args != nil {
				rt.Args = *params.Args
			}
			if params.Env != nil {
				rt.Env = map[string]string{}
				for key, value := range params.Env {
					rt.Env[key] = os.ExpandEnv(value)
				}
			}
			if params.TimeoutMillis != nil {
				rt.TimeoutMillis = *params.TimeoutMillis
			}
			p.Package.Manifest.Runtime = &rt
		}
		out = append(out, p)
	}
	return out, nil
}
