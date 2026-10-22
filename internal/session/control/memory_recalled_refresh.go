package control

import "reasonix/internal/state/memory"

// deliveredFact is one fact whose body the session handed the model, with the
// content stamp of its own file as it read then. tracked is false when the file
// does not, at delivery time, still parse to the fact being delivered — no file
// at the fact's canonical path, or bytes an external edit already replaced — so
// only the identity read can re-check it.
type deliveredFact struct {
	fact    memory.Memory
	stamp   memory.FactStamp
	tracked bool
}

// refreshDelivered reloads the snapshot when a fact the last turn handed the
// model no longer reads the same on disk, so an edit made outside this session is
// what the next recall offers. The steady state is one read of each delivered
// fact's own file: its content digest, never a stat, decides.
func (m *memoryManager) refreshDelivered() {
	m.mu.Lock()
	delivered := m.delivered
	m.mu.Unlock()
	if len(delivered) == 0 {
		return
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	mem := m.current()
	if mem == nil {
		return
	}
	for _, held := range delivered {
		if unchangedFact(mem.Store, held) {
			continue
		}
		// Changed, gone, or unreadable: the reload settles which, and an
		// unreadable store reaches the recall as its StoreError.
		m.applyWrite(mem, "")
		return
	}
}

// unchangedFact answers from the fact's own file, read directly: a tracked fact
// is unchanged exactly when those bytes are the bytes delivered, so an external
// rewrite that preserves size and modification time still counts. A fact whose
// file is not where its name says has no tracked path, and the identity read is
// the only way to re-check it.
func unchangedFact(store memory.Store, held deliveredFact) bool {
	if held.tracked {
		return held.stamp.SameContent()
	}
	now, ok, err := store.Read(held.fact.ID)
	return err == nil && ok && sameFact(held.fact, now)
}

// recordDelivered re-baselines what the next recall must re-check: this recall's
// hits, each stamped only when its file still parses to the fact the turn hands
// over. A recall that returned no hits leaves the previous set in place — the
// model still holds those facts, so they are still the ones to re-check — and
// only a recall with hits replaces them.
func (m *memoryManager) recordDelivered(result memory.RecallResult) {
	mem := m.current()
	if mem == nil {
		return
	}
	if len(result.Hits) == 0 {
		return
	}
	delivered := make([]deliveredFact, 0, len(result.Hits))
	for _, hit := range result.Hits {
		delivered = append(delivered, stampFact(mem.Store, hit.Memory))
	}
	m.mu.Lock()
	m.delivered = delivered
	m.mu.Unlock()
}

// stampFact baselines one fact against its own file, so re-checking it next turn
// costs one read of that file rather than a store scan. It marks the fact tracked
// only when that file still parses to the very fact being delivered: a fact
// edited between the snapshot load and its first recall digests bytes the turn
// never carried, so it stays untracked and the identity read decides instead.
func stampFact(store memory.Store, fact memory.Memory) deliveredFact {
	stamp, ok := store.StampOfFact(fact)
	if !ok {
		return deliveredFact{fact: fact}
	}
	current, ok := memory.FactAt(stamp.Path)
	if !ok || !sameFact(fact, current) {
		return deliveredFact{fact: fact}
	}
	return deliveredFact{fact: fact, stamp: stamp, tracked: true}
}

// appendDelivered adds the facts a write note just handed the model, one entry
// per file: a fact saved twice is still one file to re-check.
func appendDelivered(delivered, add []deliveredFact) []deliveredFact {
	for _, fact := range add {
		replaced := false
		for i := range delivered {
			if delivered[i].sameFile(fact) {
				delivered[i], replaced = fact, true
				break
			}
		}
		if !replaced {
			delivered = append(delivered, fact)
		}
	}
	return delivered
}

// sameFile reports that two delivered facts were checked at the same path, which
// only a pair the store could actually stamp can say.
func (h deliveredFact) sameFile(other deliveredFact) bool {
	return h.tracked && other.tracked && h.stamp.Path == other.stamp.Path
}

func sameFact(a, b memory.Memory) bool {
	return a.ID == b.ID && a.Revision == b.Revision && a.Name == b.Name && a.Title == b.Title &&
		a.Description == b.Description && a.Type == b.Type && a.Scope == b.Scope &&
		a.Activation == b.Activation && a.Volatility == b.Volatility && a.SubjectKey == b.SubjectKey &&
		a.ExpiresAt.Equal(b.ExpiresAt) && a.LastVerifiedAt.Equal(b.LastVerifiedAt) &&
		a.Keywords == b.Keywords && a.Body == b.Body
}
