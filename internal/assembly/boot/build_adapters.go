package boot

import (
	"time"

	"reasonix/internal/runtime/delegation"
	"reasonix/internal/session/control"
)

func (b *builder) clock() func() time.Time {
	if b.opts.clock != nil {
		return b.opts.clock
	}
	return time.Now
}

func subagentReader(s *delegation.SubagentStore) control.SubagentReader {
	if s == nil {
		return nil
	}
	return s
}
