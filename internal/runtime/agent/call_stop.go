package agent

import (
	"context"
	"errors"
	"sync"

	"reasonix/internal/contract/tool"
)

var errCallStopped = errors.New("call stopped by user")

// callStops holds the running calls a user may stop one at a time, by call id.
type callStops struct {
	mu   sync.Mutex
	stop map[string]context.CancelCauseFunc
}

// begin derives the context one call runs under and registers it; the returned
// func unregisters it and releases the context.
func (s *callStops) begin(ctx context.Context, id string) (context.Context, func()) {
	cctx, cancel := context.WithCancelCause(ctx)
	s.mu.Lock()
	if s.stop == nil {
		s.stop = map[string]context.CancelCauseFunc{}
	}
	s.stop[id] = cancel
	s.mu.Unlock()
	return cctx, func() {
		s.mu.Lock()
		delete(s.stop, id)
		s.mu.Unlock()
		cancel(nil)
	}
}

func (s *callStops) cancel(id string) bool {
	s.mu.Lock()
	cancel, ok := s.stop[id]
	s.mu.Unlock()
	if ok {
		cancel(errCallStopped)
	}
	return ok
}

// CancelToolCall stops one running call without ending the turn. It reports
// whether the call was running.
func (a *Agent) CancelToolCall(id string) bool { return a.stops.cancel(id) }

// stoppedByUser turns a failure caused by CancelToolCall into the refusal the
// model reads; a cancelled turn keeps its own error.
func stoppedByUser(turn, call context.Context, err error) error {
	if err == nil || turn.Err() != nil || !errors.Is(context.Cause(call), errCallStopped) {
		return err
	}
	return tool.Refusal{
		Code:    tool.CodeCancelledByUser,
		Message: "The user stopped this call before it finished. Anything it had already done outside the workspace is unknown; check before relying on it.",
	}
}
