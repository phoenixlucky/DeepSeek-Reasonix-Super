package persona

import "reasonix/internal/contract/config"

// Settings is a persona resolved for composition: the config value normalized,
// with the tri-state runtime-context pointer already resolved to a bool.
type Settings struct {
	Prefix                string
	Suffix                string
	IncludeRuntimeContext bool
}

// FromConfig resolves the stored persona. Normalization here is what makes an
// old config (no persona at all) compose exactly like one with an empty table.
func FromConfig(c config.PersonaConfig) Settings {
	p := c.Normalize()
	return Settings{
		Prefix:                p.Prefix,
		Suffix:                p.Suffix,
		IncludeRuntimeContext: p.RuntimeContextEnabled(),
	}
}

// ApplyPrefix puts the identity ahead of the first-party guidance.
func ApplyPrefix(base string, s Settings) string {
	if s.Prefix == "" {
		return base
	}
	if base == "" {
		return s.Prefix
	}
	return s.Prefix + "\n\n" + base
}

// ApplySuffix trails the first-party guidance with the user's suffix.
func ApplySuffix(base string, s Settings) string {
	if s.Suffix == "" {
		return base
	}
	if base == "" {
		return s.Suffix
	}
	return base + "\n\n" + s.Suffix
}
