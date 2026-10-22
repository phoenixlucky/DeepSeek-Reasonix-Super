package checkpoint

import (
	"log/slog"
	"sort"
)

// markUnreadable reserves the number of a turn file that exists but did not
// load. Its payload is unrecoverable, so the number must never be handed to a
// new turn: that would overwrite the only copy of the original snapshot.
func (s *Store) markUnreadable(turn int, path string, err error) {
	if s.unreadable == nil {
		s.unreadable = map[int]bool{}
	}
	s.unreadable[turn] = true
	if err != nil {
		slog.Warn("checkpoint: turn file unreadable; its number stays reserved", "turn", turn, "path", path, "err", err)
		return
	}
	slog.Warn("checkpoint: turn file is not a checkpoint; its number stays reserved", "turn", turn, "path", path)
}

// unreadableFloor is one past the highest reserved turn (0 when none).
func (s *Store) unreadableFloor() int {
	floor := 0
	for turn := range s.unreadable {
		if turn >= floor {
			floor = turn + 1
		}
	}
	return floor
}

// UnreadableTurns returns, ascending, the turn numbers whose persisted file
// exists but could not be read or parsed. Their payloads are lost, but the
// numbers stay reserved so no later turn can overwrite the original file.
func (s *Store) UnreadableTurns() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, 0, len(s.unreadable))
	for turn := range s.unreadable {
		out = append(out, turn)
	}
	sort.Ints(out)
	return out
}
