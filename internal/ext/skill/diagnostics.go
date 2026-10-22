package skill

import (
	"fmt"
	"io"
	"sync"
)

// diagnosed remembers what a store has already said. Discovery runs again for
// every keystroke in the slash menu, and an unchanged file yields the same
// diagnostic each time; on an interactive screen writing it again overwrites the
// input line, so a store says each distinct diagnostic once.
type diagnosed struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// diagnose writes one discovery diagnostic to the store's writer, unless it has
// already written exactly this text. A pass whose output is discarded (a
// read-only inspection) neither writes nor uses up the diagnostic.
func (s *Store) diagnose(format string, args ...any) {
	if s == nil || s.stderr == nil || s.stderr == io.Discard {
		return
	}
	text := fmt.Sprintf(format, args...)
	s.said.mu.Lock()
	defer s.said.mu.Unlock()
	if _, dup := s.said.seen[text]; dup {
		return
	}
	if s.said.seen == nil {
		s.said.seen = map[string]struct{}{}
	}
	s.said.seen[text] = struct{}{}
	_, _ = io.WriteString(s.stderr, text)
}
