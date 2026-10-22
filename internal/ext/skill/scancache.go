package skill

import (
	"slices"
	"sync"
	"time"
)

const scanTTL = 2 * time.Second

type scanKey struct {
	dir   string
	scope Scope
	flat  bool
	agent bool
}

type scanEntry struct {
	at     time.Time
	skills []Skill
}

// scanCache keeps one discovery pass per root for scanTTL, so a burst of slash
// completions reads each skill file once. The store's own writers drop it;
// edits made outside the process show up when the window closes.
type scanCache struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[scanKey]scanEntry
	gen     uint64
}

func (c *scanCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *scanCache) get(k scanKey) ([]Skill, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok || c.clock().Sub(e.at) >= scanTTL {
		return nil, false
	}
	return slices.Clone(e.skills), true
}

func (c *scanCache) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

func (c *scanCache) put(k scanKey, skills []Skill, gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
	if c.entries == nil {
		c.entries = map[scanKey]scanEntry{}
	}
	c.entries[k] = scanEntry{at: c.clock(), skills: slices.Clone(skills)}
}

func (c *scanCache) drop() {
	c.mu.Lock()
	c.entries = nil
	c.gen++
	c.mu.Unlock()
}

func (s *Store) discoverRootCached(r discoveryRoot) []Skill {
	k := scanKey{dir: r.Dir, scope: r.Scope, flat: r.requireFlatMarker, agent: r.forceSubagent}
	if out, ok := s.scans.get(k); ok {
		return out
	}
	gen := s.scans.generation()
	out := s.discoverRoot(r)
	s.scans.put(k, out, gen)
	return out
}
