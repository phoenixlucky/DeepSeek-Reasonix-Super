package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
)

const (
	// ErrRequestCancelled is the JSON-RPC code of a request the peer cancelled
	// with $/cancel_request.
	ErrRequestCancelled = -32800

	cancelRequestMethod = "$/cancel_request"
)

// flightSet is the requests being served, so one can be cancelled by its id
// without touching the others or the session they belong to.
type flightSet struct {
	mu sync.Mutex
	m  map[string]*flight
}

type flight struct {
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

func flightKey(id json.RawMessage) string { return string(bytes.TrimSpace(id)) }

// begin registers a request and returns the context its handler runs under, the
// flight to ask whether it was cancelled, and the function that ends it.
func (s *flightSet) begin(ctx context.Context, id json.RawMessage) (context.Context, *flight, func()) {
	ctx, cancel := context.WithCancel(ctx)
	f := &flight{cancel: cancel}
	key := flightKey(id)
	s.mu.Lock()
	if s.m == nil {
		s.m = map[string]*flight{}
	}
	s.m[key] = f
	s.mu.Unlock()
	return ctx, f, func() {
		s.mu.Lock()
		if s.m[key] == f {
			delete(s.m, key)
		}
		s.mu.Unlock()
		cancel()
	}
}

// cancel ends the request with this id if it is still being served. An id that
// is unknown, already answered or malformed is ignored: a notification has nowhere
// to report anything.
func (s *flightSet) cancel(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &p) != nil || len(p.RequestID) == 0 {
		return
	}
	s.mu.Lock()
	f := s.m[flightKey(p.RequestID)]
	s.mu.Unlock()
	if f != nil {
		f.cancelled.Store(true)
		f.cancel()
	}
}
