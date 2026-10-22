package event

import "strings"

// MaxTaskLabel bounds what a row has to carry; the frontend truncates to fit.
const MaxTaskLabel = 120

// TaskLabel names a sub-agent from its own task: the description the model wrote
// to say what runs, else the first line of what it was asked.
func TaskLabel(description, prompt string) string {
	for _, text := range []string{description, prompt} {
		for line := range strings.SplitSeq(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if r := []rune(line); len(r) > MaxTaskLabel {
				return string(r[:MaxTaskLabel-1]) + "…"
			}
			return line
		}
	}
	return ""
}
