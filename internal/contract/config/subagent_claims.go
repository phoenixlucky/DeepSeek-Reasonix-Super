package config

import "strings"

// SubagentClaimsOptimistic reports whether a sub-agent that declared no
// write_paths reserves nothing up front and holds only the path of the write
// in progress. Any other value, including a misspelt one, is the strict
// default: such a writer claims the whole workspace.
func (c *Config) SubagentClaimsOptimistic() bool {
	return c != nil && strings.EqualFold(strings.TrimSpace(c.Agent.SubagentClaims), "optimistic")
}

// SetSubagentClaims records the user's choice: optimistic, or the strict
// default that an undeclared writer holds the whole workspace.
func (c *Config) SetSubagentClaims(optimistic bool) {
	if optimistic {
		c.Agent.SubagentClaims = "optimistic"
		return
	}
	c.Agent.SubagentClaims = "strict"
}
