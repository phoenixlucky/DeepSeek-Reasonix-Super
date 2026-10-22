package config

import (
	"fmt"
	"strings"
)

// DefaultPlanMode reports whether a new conversation starts in plan mode. Any
// other value, including a misspelt one, is the default: it starts normally.
func (c *Config) DefaultPlanMode() bool {
	return c != nil && strings.EqualFold(strings.TrimSpace(c.Agent.DefaultMode), "plan")
}

// SetDefaultMode records how new conversations start: "normal" or "plan". A goal
// needs an objective, so it cannot be a default.
func (c *Config) SetDefaultMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "normal":
		c.Agent.DefaultMode = "normal"
	case "plan":
		c.Agent.DefaultMode = "plan"
	default:
		return fmt.Errorf("default_mode %q: want normal or plan", mode)
	}
	return nil
}
