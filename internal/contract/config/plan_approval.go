package config

import "strings"

// PlanApprovalAuto reports whether the plan card answers itself. Any other
// value, including a misspelt one, is the default: the user is asked.
func (c *Config) PlanApprovalAuto() bool {
	return c != nil && strings.EqualFold(strings.TrimSpace(c.Agent.PlanApproval), "auto")
}

// SetPlanApproval records the user's choice: auto, or the default of asking.
func (c *Config) SetPlanApproval(auto bool) {
	if auto {
		c.Agent.PlanApproval = "auto"
		return
	}
	c.Agent.PlanApproval = "ask"
}
