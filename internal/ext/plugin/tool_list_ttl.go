package plugin

import "time"

// A server that cannot announce changes says how long its tool list holds
// instead: ttlMs on the list response. The hint is a server's word, so it is
// clamped: too short would turn it into a poll, too long would pin a stale
// catalog for a whole session. A ttlMs of zero or less asks for no caching and
// adds no timer; changes still arrive through list_changed where it is declared.
const (
	minToolListTTL = 30 * time.Second
	maxToolListTTL = time.Hour
)

// ttlRefresh is the timer that re-reads the catalog when its hint runs out.
type ttlRefresh struct {
	after func(time.Duration, func()) func()
	last  time.Duration
	stop  func()
}

func clampToolListTTL(ms int64) (time.Duration, bool) {
	if ms <= 0 {
		return 0, false
	}
	// Compared in milliseconds: converting a huge hint to a Duration first
	// would wrap it around to the shortest value.
	if ms >= maxToolListTTL.Milliseconds() {
		return maxToolListTTL, true
	}
	return max(time.Duration(ms)*time.Millisecond, minToolListTTL), true
}

// noteToolListTTL records a successful listing's hint and arms the re-read.
func (c *Client) noteToolListTTL(ms *int64) {
	if ms == nil {
		return
	}
	d, ok := clampToolListTTL(*ms)
	c.refresh.mu.Lock()
	defer c.refresh.mu.Unlock()
	c.refresh.ttl.last = 0
	c.armToolListTTLLocked(d, ok)
}

// retryToolListTTL arms the re-read again after one that failed, with the hint
// it was armed with: with no notices to start a new cycle, a single failed
// read would otherwise leave the catalog stale for good.
func (c *Client) retryToolListTTL() {
	c.refresh.mu.Lock()
	defer c.refresh.mu.Unlock()
	if last := c.refresh.ttl.last; last > 0 {
		c.armToolListTTLLocked(last, true)
	}
}

func (c *Client) armToolListTTLLocked(d time.Duration, ok bool) {
	if c.refresh.ttl.stop != nil {
		c.refresh.ttl.stop()
		c.refresh.ttl.stop = nil
	}
	if !ok || c.refresh.closed {
		return
	}
	after := c.refresh.ttl.after
	if after == nil {
		after = func(d time.Duration, f func()) func() {
			timer := time.AfterFunc(d, f)
			return func() { timer.Stop() }
		}
	}
	c.refresh.ttl.last = d
	c.refresh.ttl.stop = after(d, c.requestToolsRefresh)
}

// stopToolListTTL cancels the timer; the caller holds refresh.mu.
func (c *Client) stopToolListTTLLocked() {
	if c.refresh.ttl.stop != nil {
		c.refresh.ttl.stop()
		c.refresh.ttl.stop = nil
	}
}
