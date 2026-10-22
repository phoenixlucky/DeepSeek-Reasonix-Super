package config

import "strings"

// Turn narration levels.
const (
	TurnNarrationBrief    = "brief"
	TurnNarrationDetailed = "detailed"
)

// NormalizeTurnNarration returns brief or detailed; anything else is brief.
func NormalizeTurnNarration(level string) string {
	if strings.ToLower(strings.TrimSpace(level)) == TurnNarrationDetailed {
		return TurnNarrationDetailed
	}
	return TurnNarrationBrief
}

// TurnNarration normalizes agent.turn_narration.
func (c *Config) TurnNarration() string {
	if c == nil {
		return TurnNarrationBrief
	}
	return NormalizeTurnNarration(c.Agent.TurnNarration)
}
