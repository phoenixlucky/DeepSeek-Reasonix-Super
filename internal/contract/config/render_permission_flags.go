package config

import "strings"

// renderPermissionFlags writes the two opt-in switches of [permissions]. The
// commented form (full) shows the key when it is off; the delta form writes only
// what is on.
func renderPermissionFlags(b *strings.Builder, c *Config, full bool) {
	p := c.Permissions
	switch {
	case p.AllowDynamicBash && full:
		b.WriteString("allow_dynamic_bash = true   # advanced: let mode=allow cover command substitution and interpreter -c/-e\n")
	case p.AllowDynamicBash:
		b.WriteString("allow_dynamic_bash = true\n")
	case full:
		b.WriteString("# allow_dynamic_bash = false   # advanced opt-in; deny/ask and exact rules still take precedence\n")
	}
	switch {
	case p.AskOnExternalOrigin && full:
		b.WriteString("ask_on_external_origin = true   # ask before a call that repeats text a web page or tool result gave the task, even in auto and YOLO\n")
	case p.AskOnExternalOrigin:
		b.WriteString("ask_on_external_origin = true\n")
	case full:
		b.WriteString("# ask_on_external_origin = false   # opt-in: ask before a call that repeats text an external result gave the task\n")
	}
}
