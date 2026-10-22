package acp

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync"
	"sync/atomic"
)

// messageIDs makes the opaque ids that tell a client which message chunks belong
// together. A live message is one sampling attempt, so a retried attempt is a new
// message; a replayed one is numbered. The prefix is random per session open, so
// ids from before a restart cannot collide with new ones.
type messageIDs struct {
	once   sync.Once
	prefix string
	replay atomic.Int64
}

func (m *messageIDs) base() string {
	m.once.Do(func() {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		m.prefix = hex.EncodeToString(b)
	})
	return m.prefix
}

// live is the id of the message an attempt is streaming; no attempt, no id.
func (m *messageIDs) live(attempt string) string {
	if attempt == "" {
		return ""
	}
	return m.base() + "-" + attempt
}

// replayed is the id of the next message of a replayed history.
func (m *messageIDs) replayed() string {
	return m.base() + "-r" + strconv.FormatInt(m.replay.Add(1), 10)
}

// liveMessageID is the id of the message the sink is streaming now.
func (s *updateSink) liveMessageID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.msgIDs.live(s.activeAttemptID)
}
