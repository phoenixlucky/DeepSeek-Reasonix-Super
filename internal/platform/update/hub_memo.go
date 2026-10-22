package update

import (
	"context"
	"sync"
	"time"
)

// hubFresh and hubFailedFresh bound how long one catalog read answers every
// caller: the Settings list, the versions panel and the update watcher each
// ask on their own, and an unreachable mirror made each of them wait out the
// whole catalogTimeout. A failure is remembered for far less than a success so
// a flaky link recovers on the next look.
const (
	hubFresh       = 5 * time.Minute
	hubFailedFresh = 20 * time.Second
)

type hubMemo struct {
	mu     sync.Mutex
	key    string
	at     time.Time
	res    VersionHub
	flight *hubFlight
}

type hubFlight struct {
	done chan struct{}
	res  VersionHub
}

var memo hubMemo

// get answers from the last read of the same install and pin while it is
// fresh, joins a read already under way, and otherwise reads. refresh skips
// the stored answer and still joins a read in flight.
func (m *hubMemo) get(ctx context.Context, key string, refresh bool, now func() time.Time, read func(context.Context) VersionHub) VersionHub {
	m.mu.Lock()
	if m.key == key && !refresh && !m.at.IsZero() {
		ttl := hubFresh
		if m.res.Err != "" {
			ttl = hubFailedFresh
		}
		if now().Sub(m.at) < ttl {
			res := m.res
			m.mu.Unlock()
			return res
		}
	}
	if f := m.flight; f != nil {
		m.mu.Unlock()
		select {
		case <-f.done:
			return f.res
		case <-ctx.Done():
			return read(ctx)
		}
	}
	f := &hubFlight{done: make(chan struct{})}
	m.flight = f
	m.mu.Unlock()

	res := read(ctx)

	m.mu.Lock()
	if ctx.Err() == nil {
		m.key, m.at, m.res = key, now(), res
	}
	m.flight = nil
	m.mu.Unlock()
	f.res = res
	close(f.done)
	return res
}
