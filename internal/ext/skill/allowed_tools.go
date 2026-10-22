package skill

import (
	"strings"

	"reasonix/internal/base/rulelist"
)

// parseAllowedTools splits an `allowed-tools` value into rules: commas or
// whitespace separate them, and a separator inside parentheses stays in its
// rule. Unbalanced parentheses fall back to a plain comma split; nil when absent.
func parseAllowedTools(raw string) []string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	rules, err := rulelist.Split(raw)
	if err != nil {
		return parseCSVFrontmatter(raw)
	}
	var out []string
	for _, r := range rules {
		if r = strings.Trim(r, `"'`); r != "" {
			out = append(out, r)
		}
	}
	return out
}
