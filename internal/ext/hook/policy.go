package hook

// IsPolicyGuard identifies hooks a composition cannot deselect. Permission
// hooks remain policy even when their native payload format only notifies.
func (h ResolvedHook) IsPolicyGuard() bool {
	return IsBlocking(h.Event) || h.Event == PermissionRequest
}

// ReplacePolicyGuards updates live policy without replacing retained automation.
func (r *Runner) ReplacePolicyGuards(live []ResolvedHook) {
	if r == nil {
		return
	}
	r.set.mu.Lock()
	defer r.set.mu.Unlock()
	var next []ResolvedHook
	for _, h := range r.set.hooks {
		if !h.IsPolicyGuard() {
			next = append(next, h)
		}
	}
	for _, h := range live {
		if h.IsPolicyGuard() {
			next = append(next, h)
		}
	}
	r.set.hooks, r.set.lastOutcome = next, nil
}
