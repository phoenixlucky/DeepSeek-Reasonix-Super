package tool

import "maps"

// BindMCPRuntime applies execution authority to existing and future adapters.
func (r *Registry) BindMCPRuntime(bind func(Tool) Tool) {
	r.mu.Lock()
	r.bindMCP = bind
	held := maps.Clone(r.tools)
	r.mu.Unlock()
	for _, t := range held {
		if _, ok := t.(MCPMetadata); ok {
			r.Add(t)
		}
	}
}

// Restrict fixes the ceiling on what the registry may hold. Tools already
// registered that admit rejects are removed, and every later Add is checked
// against it. admit runs outside the registry lock: a lazy tool's own methods
// may take a lock that a registry swap holds while it waits for this one.
func (r *Registry) Restrict(admit func(Tool) bool) {
	if r == nil || admit == nil {
		return
	}
	r.mu.Lock()
	r.admit = admit
	held := maps.Clone(r.tools)
	r.mu.Unlock()
	rejected := map[string]bool{}
	for name, t := range held {
		if t != nil && !admit(t) {
			rejected[name] = true
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.order[:0]
	for _, name := range r.order {
		if rejected[name] {
			delete(r.tools, name)
			delete(r.canon, name)
			continue
		}
		kept = append(kept, name)
	}
	r.order = kept
	r.schemaRev.Add(1)
}

// ShowToProvider adds or removes names from an existing provider-visible
// restriction; with no restriction every registered tool is already shown.
func (r *Registry) ShowToProvider(shown bool, names ...string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providerVisible == nil {
		return
	}
	for _, name := range names {
		if r.providerVisible[name] == shown {
			continue
		}
		if shown {
			r.providerVisible[name] = true
		} else {
			delete(r.providerVisible, name)
		}
		r.schemaRev.Add(1)
	}
}
