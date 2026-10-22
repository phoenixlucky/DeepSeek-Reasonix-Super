package plugin

import (
	"math"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestToolListTTLIsClampedAndZeroMeansNoTimer(t *testing.T) {
	for _, tc := range []struct {
		ms   int64
		want time.Duration
		ok   bool
	}{
		{0, 0, false},
		{-5, 0, false},
		{1, minToolListTTL, true},
		{45_000, 45 * time.Second, true},
		{24 * 3600 * 1000, maxToolListTTL, true},
		{maxToolListTTL.Milliseconds(), maxToolListTTL, true},
		{9_223_372_036_855, maxToolListTTL, true},
		{1 << 62, maxToolListTTL, true},
		{math.MaxInt64, maxToolListTTL, true},
	} {
		got, ok := clampToolListTTL(tc.ms)
		if got != tc.want || ok != tc.ok {
			t.Errorf("clamp(%d) = %v, %v; want %v, %v", tc.ms, got, ok, tc.want, tc.ok)
		}
	}
}

// fakeTimers records what the client armed and lets a test run it by hand.
type fakeTimers struct {
	mu      sync.Mutex
	armed   []time.Duration
	pending []func()
	stopped int
}

func (f *fakeTimers) after(d time.Duration, fn func()) func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.armed = append(f.armed, d)
	f.pending = append(f.pending, fn)
	return func() {
		f.mu.Lock()
		f.stopped++
		f.mu.Unlock()
	}
}

func (f *fakeTimers) armedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.armed)
}

func (f *fakeTimers) fireLast() {
	f.mu.Lock()
	fn := f.pending[len(f.pending)-1]
	f.mu.Unlock()
	fn()
}

func connectedClient(t *testing.T, h *Host) *Client {
	t.Helper()
	h.mu.RLock()
	defer h.mu.RUnlock()
	c := h.lookupClientLocked("live")
	if c == nil {
		t.Fatal("no connected client")
	}
	return c
}

func withFakeTimers(t *testing.T, c *Client) *fakeTimers {
	t.Helper()
	f := &fakeTimers{}
	c.refresh.mu.Lock()
	c.refresh.ttl.after = f.after
	c.refresh.mu.Unlock()
	return f
}

// A server that cannot announce changes says how long its list holds. When the
// hint runs out the catalog is read again and a changed one is republished.
func TestToolListTTLRepublishesACatalogTheServerCannotAnnounce(t *testing.T) {
	ms := int64(60_000)
	server, srv := newListChangedServer(t, false, "alpha")
	server.ttlMs = &ms
	h, published := connectListChanged(t, srv, Spec{})
	c := connectedClient(t, h)
	timers := withFakeTimers(t, c)

	c.noteToolListTTL(&ms)
	if got := timers.armed; !slices.Equal(got, []time.Duration{time.Minute}) {
		t.Fatalf("armed = %v, want one timer at the server's hint", got)
	}
	server.announce(0, "alpha", "gamma")
	timers.fireLast()
	if got := waitForCatalog(t, published); !slices.Equal(got, []string{"mcp__live__alpha", "mcp__live__gamma"}) {
		t.Fatalf("republished = %v", got)
	}
	if timers.armedCount() != 2 {
		t.Fatalf("armed %d times, want the re-read to arm the next one", timers.armedCount())
	}
}

func TestToolListWithoutAUsableHintArmsNoTimer(t *testing.T) {
	zero := int64(0)
	for name, hint := range map[string]*int64{"absent": nil, "zero": &zero} {
		t.Run(name, func(t *testing.T) {
			server, srv := newListChangedServer(t, false, "alpha")
			server.ttlMs = hint
			h, _ := connectListChanged(t, srv, Spec{})
			c := connectedClient(t, h)
			timers := withFakeTimers(t, c)
			c.noteToolListTTL(hint)
			if timers.armedCount() != 0 {
				t.Fatalf("armed %v for a hint of %v", timers.armed, hint)
			}
		})
	}
}

// Nothing announces a change on such a server, so a failed re-read has to
// schedule the next one itself or the catalog stays stale for the session.
func TestToolListTTLRetriesAfterAFailedRead(t *testing.T) {
	ms := int64(60_000)
	server, srv := newListChangedServer(t, false, "alpha")
	server.ttlMs = &ms
	h, published := connectListChanged(t, srv, Spec{})
	c := connectedClient(t, h)
	timers := withFakeTimers(t, c)
	c.noteToolListTTL(&ms)

	server.failList.Store(true)
	timers.fireLast()
	deadline := time.Now().Add(10 * time.Second)
	for timers.armedCount() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if timers.armedCount() < 2 {
		t.Fatal("a failed re-read left no timer behind")
	}
	server.failList.Store(false)
	server.announce(0, "alpha", "delta")
	timers.fireLast()
	if got := waitForCatalog(t, published); !slices.Contains(got, "mcp__live__delta") {
		t.Fatalf("republished = %v", got)
	}
}

func TestClosingAConnectionStopsItsTimer(t *testing.T) {
	ms := int64(60_000)
	_, srv := newListChangedServer(t, false, "alpha")
	h, _ := connectListChanged(t, srv, Spec{})
	c := connectedClient(t, h)
	timers := withFakeTimers(t, c)
	c.noteToolListTTL(&ms)
	c.stopToolListWatch()
	timers.mu.Lock()
	stopped := timers.stopped
	timers.mu.Unlock()
	if stopped != 1 {
		t.Fatalf("stopped = %d, want the armed timer cancelled", stopped)
	}
	c.noteToolListTTL(&ms)
	if timers.armedCount() != 1 {
		t.Fatal("a closed connection armed a new timer")
	}
}
