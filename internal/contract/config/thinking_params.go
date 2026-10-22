package config

import "strings"

// reasoningProtocolNone is the request shape that carries no thinking or
// reasoning control fields at all.
const reasoningProtocolNone = "none"

// CanConfigureThinkingParams reports whether reasoning_protocol governs this
// entry's request shape, which the protocol catalog decides.
func CanConfigureThinkingParams(e *ProviderEntry) bool {
	if e == nil {
		return false
	}
	p, ok := ProtocolFor(e.Kind)
	return ok && p.ReasoningParams
}

// SendsThinkingParams reports whether this endpoint may receive thinking
// controls. False means the user pinned the plain-chat shape because the
// gateway rejects those fields.
func SendsThinkingParams(e *ProviderEntry) bool {
	if !CanConfigureThinkingParams(e) {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(e.ReasoningProtocol), reasoningProtocolNone)
}
