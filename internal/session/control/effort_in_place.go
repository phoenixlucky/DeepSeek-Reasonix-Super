package control

import (
	"slices"
	"strings"
	"sync/atomic"
)

// effortSelection is the runtime selection a session reached by changing depth in
// place; unset means the one the controller was built with.
type effortSelection struct {
	sel   atomic.Pointer[RuntimeSelection]
	level atomic.Pointer[string] // the depth as the user named it, for a status line
}

// EffortInPlacer is the controller capability a frontend uses to change reasoning
// depth without a rebuild; a frontend falls back to its rebuild when it answers no.
type EffortInPlacer interface {
	EffortInPlaceEligible(level string) bool
	SetEffortInPlace(level string, target RuntimeSelection) bool
}

var _ EffortInPlacer = (*Controller)(nil)

// EffortInPlaceEligible reports whether level can be applied to the running
// session without rebuilding it: the provider takes that depth on each request.
// "auto" and anything the provider does not list rebuild, since an override can
// only choose among depths, not remove the effort the provider was built with.
func (c *Controller) EffortInPlaceEligible(level string) bool {
	level = strings.ToLower(strings.TrimSpace(level))
	if c == nil || c.executor == nil || level == "" || level == "auto" {
		return false
	}
	return slices.ContainsFunc(c.executor.RequestEffortLevels(), func(l string) bool {
		return strings.EqualFold(strings.TrimSpace(l), level)
	})
}

// SetEffortInPlace applies level to later requests of this session and records
// target as the running selection, so a frontend comparing a requested selection
// with the running one sees a no-op. It reports false, changing nothing, when the
// level needs a rebuild. It rides the request, so it is accepted while a turn
// runs and takes effect on the next model call; the cached prefix is untouched.
func (c *Controller) SetEffortInPlace(level string, target RuntimeSelection) bool {
	if !c.EffortInPlaceEligible(level) {
		return false
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if level == normalizeRuntimeEffort(c.effort) {
		c.executor.SetRequestEffort("")
	} else {
		c.executor.SetRequestEffort(level)
	}
	target.ModelRef = strings.TrimSpace(c.modelRef)
	c.effortSel.sel.Store(&target)
	c.effortSel.level.Store(&level)
	return true
}

// runtimeSelectionNow is the selection the session runs under now.
func (c *Controller) runtimeSelectionNow() RuntimeSelection {
	if p := c.effortSel.sel.Load(); p != nil {
		return *p
	}
	return RuntimeSelection{
		ModelRef:            strings.TrimSpace(c.modelRef),
		Effort:              normalizeRuntimeEffort(c.effort),
		ProviderFingerprint: strings.TrimSpace(c.providerFingerprint),
	}
}
