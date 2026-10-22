package config

import (
	"fmt"
	"strings"
)

// EgressUnlistedAllow is the one value that lets a host outside allowed_domains
// through without asking.
const EgressUnlistedAllow = "allow"

// EgressUnlistedAllowed reports the user's choice to stop being asked about
// hosts the allow list does not name. Any other value, including a misspelt
// one, keeps the question. Denied hosts and the proxy's address checks are not
// part of what it skips.
func (c *Config) EgressUnlistedAllowed() bool {
	return c != nil && strings.EqualFold(strings.TrimSpace(c.Sandbox.EgressUnlisted), EgressUnlistedAllow)
}

func renderDomainLists(b *strings.Builder, s SandboxConfig) {
	if len(s.DeniedDomains) > 0 {
		fmt.Fprintf(b, "denied_domains = %s\n", renderStringArray(s.DeniedDomains))
	}
	if s.EgressUnlisted != "" {
		fmt.Fprintf(b, "egress_unlisted = %q   # allow: hosts outside allowed_domains pass without asking; denied_domains still refuse; user file only\n", s.EgressUnlisted)
	}
}
