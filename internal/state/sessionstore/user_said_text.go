package sessionstore

import "strings"

// planModeMarkerPrefix opens the plan-mode contract the host prepends to a
// planning turn; the person's words follow the closing bracket.
const planModeMarkerPrefix = "[Plan mode"

// userAuthoredText is the definition every reader of a persisted turn shares:
// the host's prepended and appended blocks and the legacy compiler/delivery
// wrappers are not the person's words. UserPreviewText and UserSaidText call it.
func userAuthoredText(content string) string {
	s := StripTransientUserBlocks(content)
	s = HandoffTask(s)
	s = StripTransientUserBlocks(s)
	return strings.TrimSpace(s)
}

// UserSaidText is the one definition of what a persisted user turn says: the
// host's prepended and appended blocks and the plan-mode marker are not the
// person's words. The history index, its snippets and its recall reader call it.
func UserSaidText(content string) string {
	trimmed := userAuthoredText(content)
	if rest, ok := strings.CutPrefix(trimmed, planModeMarkerPrefix); ok {
		if _, after, ok := strings.Cut(rest, "]"); ok {
			return strings.TrimSpace(after)
		}
	}
	return trimmed
}
