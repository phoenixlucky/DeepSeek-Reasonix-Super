package acp

import "reasonix/internal/session/control"

// effortAppliedInPlace applies a depth change to the running session when it is
// the only axis changing and the provider takes a depth on each request: no
// rebuild, and nothing to queue behind a busy turn since the next model call
// carries it. It reports false, changing nothing, when the change needs a rebuild.
func effortAppliedInPlace(sess *acpSession, cfgState SessionConfigState, deltas []sessionConfigDelta) bool {
	if sess == nil || len(deltas) != 1 || deltas[0].axis != "thought_level" {
		return false
	}
	sess.mu.Lock()
	if sess.deleted || sess.maintenanceDone != nil {
		sess.mu.Unlock()
		return false
	}
	ctrl := sess.ctrl
	sess.mu.Unlock()
	placer, ok := ctrl.(control.EffortInPlacer)
	if !ok {
		return false
	}
	if running, ok := ctrl.(interface {
		RuntimeSelection() control.RuntimeSelection
	}); !ok || running.RuntimeSelection().ModelRef != cfgState.Model {
		return false
	}
	return placer.SetEffortInPlace(cfgState.ResolvedEffort, control.RuntimeSelection{
		ModelRef:            cfgState.Model,
		Effort:              cfgState.ResolvedEffort,
		ProviderFingerprint: cfgState.ProviderFingerprint,
	})
}
