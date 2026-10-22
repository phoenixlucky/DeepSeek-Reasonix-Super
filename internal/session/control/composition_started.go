package control

import (
	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

type compositionHistory interface {
	CompositionBinding() *agentcomposition.Binding
	History() []provider.Message
	Turn() int
}

func CompositionStarted(c compositionHistory) bool {
	if lightweight, ok := c.(interface {
		CompositionSummary() *agentcomposition.Summary
	}); ok {
		if summary := lightweight.CompositionSummary(); summary != nil && summary.Started {
			return true
		}
	} else {
		if binding := c.CompositionBinding(); binding != nil && binding.Started {
			return true
		}
	}
	if c.Turn() != 0 {
		return true
	}
	for _, msg := range c.History() {
		if sessionstore.ClassifyTurn(msg, 0).StartsTurn {
			return true
		}
	}
	return false
}
