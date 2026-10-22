package usecap

import (
	"sync"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/plugin"
)

type InventorySource func() ([]config.PluginEntry, []plugin.Spec, map[string]bool, error)

type inventorySource struct {
	mu          sync.Mutex
	load        InventorySource
	fingerprint string
	specs       []plugin.Spec
}

func (r *MCPCapabilityRuntime) ConfigureInventorySource(load InventorySource) {
	r.source = &inventorySource{load: load}
}

func (r *MCPCapabilityRuntime) refreshInventory() error {
	if r == nil || r.source == nil {
		return nil
	}
	source := r.source
	source.mu.Lock()
	defer source.mu.Unlock()
	entries, specs, enabled, err := source.load()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(specs))
	for _, spec := range specs {
		keys = append(keys, plugin.SchemaCacheKey(spec)+":"+spec.Package)
	}
	fingerprint := agentcomposition.Digest(struct {
		Entries []config.PluginEntry
		Keys    []string
		Enabled map[string]bool
	}{entries, keys, enabled})
	if source.fingerprint != fingerprint || !sameInventorySpecs(source.specs, specs) {
		r.ConfigureServers(entries, specs, enabled)
		source.fingerprint = fingerprint
		source.specs = make([]plugin.Spec, len(specs))
		for i, spec := range specs {
			source.specs[i] = cloneMCPSpec(spec)
		}
	}
	return nil
}

func sameInventorySpecs(previous, current []plugin.Spec) bool {
	if len(previous) != len(current) {
		return false
	}
	for i, spec := range current {
		if previous[i].Authorized != spec.Authorized || !plugin.MCPRuntimeSpecMatches(previous[i], spec) {
			return false
		}
	}
	return true
}
