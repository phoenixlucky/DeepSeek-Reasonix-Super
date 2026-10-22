package boot

import (
	"context"
	"errors"

	"reasonix/internal/ext/extension/protocol"
	"reasonix/internal/session/control"
)

// extensionSession serves an extension's session calls through the controller
// the build publishes. A call before then, or after a failed build, is
// unavailable rather than held: nothing waits on a session that may not exist.
type extensionSession struct {
	ctrl func() *control.Controller
}

func (s extensionSession) controller() (*control.Controller, error) {
	if c := s.ctrl(); c != nil {
		return c, nil
	}
	return nil, &protocol.ProtocolError{Reason: protocol.ErrUnknownMethod, Message: "the session is not ready"}
}

func (s extensionSession) ReadSession(_ context.Context, _ string, p protocol.SessionReadParams) (protocol.SessionReadResult, error) {
	c, err := s.controller()
	if err != nil {
		return protocol.SessionReadResult{}, err
	}
	out, err := c.ReadExtensionSession(p)
	if errors.Is(err, control.ErrExtensionReadKind) {
		return out, &protocol.ProtocolError{Reason: protocol.ErrInvalidParams, Message: err.Error()}
	}
	return out, err
}

func (s extensionSession) SubmitPrompt(_ context.Context, plugin string, p protocol.PromptSubmitParams) (protocol.PromptSubmitResult, error) {
	c, err := s.controller()
	if err != nil {
		return protocol.PromptSubmitResult{}, err
	}
	switch err := c.QueueExtensionPrompt(plugin, p.Text); {
	case errors.Is(err, control.ErrExtensionSubmitRate):
		return protocol.PromptSubmitResult{Refused: protocol.PromptSubmitRefusedPerTurn}, nil
	case errors.Is(err, control.ErrExtensionSubmitTooLong):
		return protocol.PromptSubmitResult{Refused: protocol.PromptSubmitRefusedTooLong}, nil
	case errors.Is(err, control.ErrInboxNeedsSession):
		return protocol.PromptSubmitResult{Refused: protocol.PromptSubmitRefusedNoSession}, nil
	case err != nil:
		return protocol.PromptSubmitResult{}, &protocol.ProtocolError{Reason: protocol.ErrInternal, Message: "the prompt could not be queued"}
	}
	return protocol.PromptSubmitResult{Queued: true}, nil
}
