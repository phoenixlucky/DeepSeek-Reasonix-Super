package boot

import (
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/ext/pluginspec"
	"reasonix/internal/runtime/usecap"
)

func (b *builder) liveMCPInventory() usecap.InventorySource {
	return func() ([]config.PluginEntry, []plugin.Spec, map[string]bool, error) {
		cfg, err := b.roots.LoadForRootReadOnly(b.root)
		if err != nil {
			return nil, nil, nil, err
		}
		current := &builder{cfg: cfg, opts: b.opts, roots: b.roots, root: b.root}
		if err := current.prepareLiveComposition(); err != nil {
			return nil, nil, nil, err
		}
		options := b.tools.specOptions
		options.PackageOwners = pluginspec.PackageOwners(cfg)
		plan := resolveMCPSpecs(current.opts, cfg, b.root, options)
		entries := append([]config.PluginEntry(nil), cfg.Plugins...)
		specs := pluginspec.ForRootWithOptions(cfg.Plugins, b.root, options)
		for _, spec := range plan.extra {
			entries = append(entries, compositionMCPEntry(spec))
			specs = append(specs, spec)
			plan.enabled[spec.Name] = true
		}
		return entries, specs, plan.enabled, nil
	}
}
