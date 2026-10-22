package config

import "strings"

// MaxGoalAutoRetry bounds how many times in a row the host retries a Goal that
// paused because its completion check could not run.
const MaxGoalAutoRetry = 5

// GoalAutoRetryLimit is the user's retry allowance, clamped to the range the
// host honours. Zero, the default, leaves the Goal paused for the user.
func (c *Config) GoalAutoRetryLimit() int {
	if c == nil {
		return 0
	}
	return min(max(c.Agent.GoalAutoRetry, 0), MaxGoalAutoRetry)
}

// renderUnattended writes the settings that let a long run go on without a click.
func renderUnattended(b *strings.Builder, c *Config) {
	renderAskTimeout(b, c)
	agentSetting(b, "goal_auto_retry", c.Agent.GoalAutoRetry, 0, "retry a Goal paused because its completion check could not run, up to this many times in a row (max 5); user file only")
}
