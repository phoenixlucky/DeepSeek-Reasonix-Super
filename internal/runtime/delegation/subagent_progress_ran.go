package delegation

import "reasonix/internal/contract/event"

// ranBook is the model and effort each child actually started with, by child id.
// Its owner (the merger) guards it with its own lock.
type ranBook map[string]*event.Profile

func (b ranBook) of(childID string) *event.Profile { return b[childID] }

// setRan records what a child's run started with, once its model and effort are
// resolved; an empty model records nothing.
func (m *subagentProgressMerger) setRan(childID, model, effort string) {
	if model == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ran == nil {
		m.ran = ranBook{}
	}
	m.ran[childID] = &event.Profile{Model: model, Effort: effort}
}

// ran records what this child's run started with, so the status events that
// follow carry it.
func (t *subagentProgressTracker) ran(model, effort string) {
	t.merger.setRan(t.childID, model, effort)
}

// identifyChild names the runtime a child resolved to on its progress events and
// returns the canonical model reference its usage is billed under.
func (t *TaskTool) identifyChild(trk *subagentProgressTracker, modelRef, effortRef string) string {
	usage := t.usageModelRef(modelRef, effortRef)
	ranModel, ranEffort := t.effectiveIdentity(modelRef, effortRef)
	trk.ran(firstNonEmpty(ranModel, usage), ranEffort)
	return usage
}
