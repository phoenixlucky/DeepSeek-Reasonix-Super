package config

import (
	"errors"
	"fmt"
)

// Bounds for what one referenced conversation may take: enough for a short
// exchange at the low end, and well inside a large window at the high end.
const (
	DefaultSessionRefTokens = 8000
	SessionRefTokensMin     = 500
	SessionRefTokensMax     = 200000
)

// SessionRefTokens is the size one referenced past conversation may take, in
// tokens, held to SessionRefTokensMin..SessionRefTokensMax; unset is the default.
func (c *Config) SessionRefTokens() int {
	if c == nil || c.Desktop.SessionRefTokens <= 0 {
		return DefaultSessionRefTokens
	}
	return min(max(c.Desktop.SessionRefTokens, SessionRefTokensMin), SessionRefTokensMax)
}

// ErrSessionRefTokensOutOfRange marks a limit outside what a referenced
// conversation may take.
var ErrSessionRefTokensOutOfRange = errors.New("session reference limit out of range")

// SetSessionRefTokens stores the limit; 0 clears it back to the default, and
// anything else must lie within SessionRefTokensMin..SessionRefTokensMax.
func (c *Config) SetSessionRefTokens(tokens int) error {
	if tokens != 0 && (tokens < SessionRefTokensMin || tokens > SessionRefTokensMax) {
		return fmt.Errorf("%w: tokens must be 0 (default) or between %d and %d, not %d", ErrSessionRefTokensOutOfRange, SessionRefTokensMin, SessionRefTokensMax, tokens)
	}
	c.Desktop.SessionRefTokens = tokens
	return nil
}
