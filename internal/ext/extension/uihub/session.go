package uihub

import (
	"context"

	"reasonix/internal/ext/extension/protocol"
)

// SessionAccess is what the host does for an extension's session calls. The
// plugin id comes from the connection the call arrived on, never from the call.
type SessionAccess interface {
	ReadSession(ctx context.Context, plugin string, p protocol.SessionReadParams) (protocol.SessionReadResult, error)
	SubmitPrompt(ctx context.Context, plugin string, p protocol.PromptSubmitParams) (protocol.PromptSubmitResult, error)
}

// live rejects a call from a plugin that is not bound, or whose sidecar died.
func (h *Hub) live(plugin string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.known[plugin] {
		return unknownClientError(plugin)
	}
	if h.crashed[plugin] {
		return &protocol.ProtocolError{Reason: protocol.ErrProviderInterrupted, Message: "extension sidecar " + plugin + " crashed"}
	}
	return nil
}

func (b binding) ReadSession(ctx context.Context, p protocol.SessionReadParams) (protocol.SessionReadResult, error) {
	if err := b.hub.live(b.pluginID); err != nil {
		return protocol.SessionReadResult{}, err
	}
	if b.hub.session == nil {
		return protocol.SessionReadResult{}, &protocol.ProtocolError{Reason: protocol.ErrUnknownMethod, Message: "extension session access is not available on this host"}
	}
	return b.hub.session.ReadSession(ctx, b.pluginID, p)
}

func (b binding) SubmitPrompt(ctx context.Context, p protocol.PromptSubmitParams) (protocol.PromptSubmitResult, error) {
	if err := b.hub.live(b.pluginID); err != nil {
		return protocol.PromptSubmitResult{}, err
	}
	if b.hub.session == nil {
		return protocol.PromptSubmitResult{}, &protocol.ProtocolError{Reason: protocol.ErrUnknownMethod, Message: "extension session access is not available on this host"}
	}
	return b.hub.session.SubmitPrompt(ctx, b.pluginID, p)
}
