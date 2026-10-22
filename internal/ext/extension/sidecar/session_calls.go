package sidecar

import (
	"context"
	"encoding/json"
	"fmt"

	"reasonix/internal/ext/extension/protocol"
)

// SessionHandler is the optional interface a UIHandler implements to serve an
// extension's session reads and prompt submissions. Without it those calls
// answer unknown_method, as UI calls do without a hub.
type SessionHandler interface {
	ReadSession(ctx context.Context, p protocol.SessionReadParams) (protocol.SessionReadResult, error)
	SubmitPrompt(ctx context.Context, p protocol.PromptSubmitParams) (protocol.PromptSubmitResult, error)
}

// sessionHandler is the handler for a call that needs a session capability:
// "session.read" for reads, "session.submit" for queued prompts. The runtime
// must have declared it, and the host must serve it.
func (c *Client) sessionHandler(method protocol.Method, capability string) (SessionHandler, error) {
	if c.rt == nil || !containsString(c.rt.Capabilities, capability) {
		return nil, &protocol.ProtocolError{Reason: protocol.ErrCapabilityNotDeclared, Message: fmt.Sprintf("extension %s called %s without the %s capability", c.pluginID, method, capability)}
	}
	h, ok := c.ui.(SessionHandler)
	if !ok {
		return nil, &protocol.ProtocolError{Reason: protocol.ErrUnknownMethod, Message: "extension session access is not available on this host"}
	}
	return h, nil
}

func (c *Client) handleSessionRead(ctx context.Context, raw json.RawMessage) (any, error) {
	decoded, err := protocol.DecodeExtensionRequestParams(protocol.MethodHostSessionRead, raw)
	if err != nil {
		return nil, protocol.MustProtocolError(protocol.ErrInvalidParams).RPCError()
	}
	h, err := c.sessionHandler(protocol.MethodHostSessionRead, "session.read")
	if err != nil {
		return nil, mapHandlerError(err)
	}
	result, err := h.ReadSession(ctx, decoded.(protocol.SessionReadParams))
	if err != nil {
		return nil, mapHandlerError(err)
	}
	return result, nil
}

func (c *Client) handlePromptSubmit(ctx context.Context, raw json.RawMessage) (any, error) {
	decoded, err := protocol.DecodeExtensionRequestParams(protocol.MethodHostPromptSubmit, raw)
	if err != nil {
		return nil, protocol.MustProtocolError(protocol.ErrInvalidParams).RPCError()
	}
	h, err := c.sessionHandler(protocol.MethodHostPromptSubmit, "session.submit")
	if err != nil {
		return nil, mapHandlerError(err)
	}
	result, err := h.SubmitPrompt(ctx, decoded.(protocol.PromptSubmitParams))
	if err != nil {
		return nil, mapHandlerError(err)
	}
	return result, nil
}
