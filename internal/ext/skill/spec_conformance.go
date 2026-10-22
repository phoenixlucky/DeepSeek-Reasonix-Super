package skill

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Limits the Agent Skills specification puts on front matter. A skill that
// breaks one still loads here; the finding reaches doctor so an author sees it
// before another agent refuses the same file.
const (
	specNameMax          = 64
	specDescriptionMax   = 1024
	specCompatibilityMax = 500
)

// specIssues reports where a skill's front matter departs from the
// specification. Only values that are present are judged: a missing name falls
// back to the directory or file name, as it always has.
func specIssues(path, stem string, fm map[string]string) []string {
	var out []string
	if raw, ok := fm[skillFrontmatterName]; ok {
		out = append(out, specNameIssues(strings.TrimSpace(raw))...)
		if filepath.Base(path) == SkillFile && strings.TrimSpace(raw) != "" && strings.TrimSpace(raw) != stem {
			out = append(out, fmt.Sprintf("name %q does not match its directory %q; the specification requires them to be the same", strings.TrimSpace(raw), stem))
		}
	}
	if raw, ok := fm[skillFrontmatterDescription]; ok {
		if n := len([]rune(strings.TrimSpace(raw))); n > specDescriptionMax {
			out = append(out, fmt.Sprintf("description is %d characters; the specification allows %d", n, specDescriptionMax))
		}
	}
	if raw, ok := fm["compatibility"]; ok {
		n := len([]rune(strings.TrimSpace(raw)))
		if n == 0 || n > specCompatibilityMax {
			out = append(out, fmt.Sprintf("compatibility is %d characters; the specification wants 1 to %d", n, specCompatibilityMax))
		}
	}
	return out
}

func specNameIssues(name string) []string {
	if name == "" {
		return nil
	}
	var out []string
	if n := len([]rune(name)); n > specNameMax {
		out = append(out, fmt.Sprintf("name is %d characters; the specification allows %d", n, specNameMax))
	}
	if name != strings.ToLower(name) {
		out = append(out, "name has uppercase letters; the specification allows lowercase only")
	}
	if strings.IndexFunc(name, func(r rune) bool { return r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r) }) >= 0 {
		out = append(out, "name has characters other than letters, digits and hyphens")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		out = append(out, "name starts or ends with a hyphen")
	}
	if strings.Contains(name, "--") {
		out = append(out, "name has consecutive hyphens")
	}
	return out
}
