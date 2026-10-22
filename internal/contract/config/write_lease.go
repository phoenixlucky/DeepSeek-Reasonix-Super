package config

import (
	"strings"
	"time"
)

// WriteLeaseOptimistic reports whether this session's cross-session write lease
// leaves a writer that declared no extent out of the whole-workspace hold. Any
// other value, including a misspelt one, is the strict default.
func (c *Config) WriteLeaseOptimistic() bool {
	return c != nil && strings.EqualFold(strings.TrimSpace(c.Agent.WriteLease), "optimistic")
}

// SetWriteLease records the user's choice: optimistic, or the strict default
// that a writer declaring no extent holds the whole workspace.
func (c *Config) SetWriteLease(optimistic bool) {
	if optimistic {
		c.Agent.WriteLease = "optimistic"
		return
	}
	c.Agent.WriteLease = "strict"
}

// WriteLeaseMaxWait is how long a write blocks behind another session's claim
// before it fails; zero, the default, waits as long as the turn lasts.
func (c *Config) WriteLeaseMaxWait() time.Duration {
	if c == nil {
		return 0
	}
	return time.Duration(max(c.Agent.WriteLeaseMaxWaitMinutes, 0)) * time.Minute
}

// SetWriteLeaseMaxWaitMinutes records the user's limit; 0 removes it.
func (c *Config) SetWriteLeaseMaxWaitMinutes(minutes int) {
	c.Agent.WriteLeaseMaxWaitMinutes = max(minutes, 0)
}
