package agent

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

const largestContextItems = 3

// contextComposition measures what holds the visible prompt's tokens, by the
// structure of the transcript: role, whether a message is a fold digest, and
// whether it belongs to the turn still running. Nothing here reads wording.
func (a *contextWindow) contextComposition() event.ContextComposition {
	visible := a.modelVisibleMessages()
	// Measured first: estimating a slice of visible clears the creation stamps
	// that mark the running turn.
	items := a.largestMessages(visible)
	var system, digest, user, reply, output []provider.Message
	for _, m := range visible {
		switch {
		case m.Role == provider.RoleSystem:
			system = append(system, m)
		case isCompactionSummary(m):
			digest = append(digest, m)
		case m.Role == provider.RoleUser:
			user = append(user, m)
		case m.Role == provider.RoleAssistant:
			reply = append(reply, m)
		case m.Role == provider.RoleTool:
			output = append(output, m)
		}
	}
	parts := []event.ContextPart{
		{Name: "system", Tokens: a.classTokens(system, false)},
		{Name: "tools", Tokens: a.classTokens(nil, true)},
		{Name: "digest", Tokens: a.classTokens(digest, false)},
		{Name: "user", Tokens: a.classTokens(user, false)},
		{Name: "reply", Tokens: a.classTokens(reply, false)},
		{Name: "tool_output", Tokens: a.classTokens(output, false)},
	}
	slices.SortStableFunc(parts, func(x, y event.ContextPart) int { return cmp.Compare(y.Tokens, x.Tokens) })
	out := event.ContextComposition{Total: a.estimatedVisibleRequestTokens(visible), At: a.compactTrigger(), Parts: parts}

	active := a.activeTurnStart(visible)
	if active >= 0 {
		out.Active = a.classTokens(visible[active:], false)
	}
	out.Largest = items[:min(largestContextItems, len(items))]
	return out
}

// largestMessages is every conversation message the prompt holds, biggest
// first, with the tool that produced a result and whether it belongs to the
// turn still running. The system prompt and fold digests are classes of their
// own, not messages a reader can act on one by one.
func (a *contextWindow) largestMessages(visible []provider.Message) []event.ContextItem {
	active := a.activeTurnStart(visible)
	items := make([]event.ContextItem, 0, len(visible))
	for i, m := range visible {
		if m.Role == provider.RoleSystem || isCompactionSummary(m) {
			continue
		}
		item := event.ContextItem{Role: string(m.Role), Tokens: fixedTokenEstimate(m), Active: active >= 0 && i >= active}
		if m.Role == provider.RoleTool {
			if j := findToolCaller(visible, i, m.ToolCallID); j >= 0 {
				item.Tool = toolCallName(visible[j], m.ToolCallID)
			}
		}
		items = append(items, item)
	}
	slices.SortStableFunc(items, func(x, y event.ContextItem) int { return cmp.Compare(y.Tokens, x.Tokens) })
	return items
}

func toolCallName(caller provider.Message, id string) string {
	for _, tc := range caller.ToolCalls {
		if tc.ID == id {
			return tc.Name
		}
	}
	return ""
}

// noteContextComposition tells the user what an over-threshold prompt is made
// of. The sentence is the English fallback; a frontend words the typed payload.
func (a *contextWindow) noteContextComposition() {
	if a == nil || a.svc.sink == nil {
		return
	}
	c := a.contextComposition()
	a.svc.sink.Emit(event.Event{
		Kind: event.Notice, Level: event.LevelWarn, Code: event.NoticeCodeContextComposition,
		Detail: c.Encode(), Text: compositionSentence(c),
	})
}

func compositionSentence(c event.ContextComposition) string {
	var top []string
	for _, p := range c.Parts[:min(3, len(c.Parts))] {
		top = append(top, fmt.Sprintf("%s %d", strings.ReplaceAll(p.Name, "_", " "), p.Tokens))
	}
	s := fmt.Sprintf("The context holds about %d tokens, over the %d compaction threshold; largest parts: %s.", c.Total, c.At, strings.Join(top, ", "))
	if c.Active > 0 {
		s += fmt.Sprintf(" The turn in progress holds %d of them, and a fold cannot take it.", c.Active)
	}
	return s
}

// remainsFloorTokens and remainsWindowShare decide when a fold's card says what
// is left: a quarter of the window, or this many tokens when no window is declared.
const (
	remainsFloorTokens = 100_000
	remainsWindowShare = 4
)

// remainsAfterFold reports what the prompt still holds once a fold has been
// installed, when that is still a large share of the window. A person looking at
// "607k to 316k" is otherwise left to wonder whether the rest is normal, and
// only the host knows what it is made of. Nil when the remainder is small.
func (a *contextWindow) remainsAfterFold() *event.ContextComposition {
	c := a.contextComposition()
	if window := a.effectiveContextWindow(); window > 0 {
		if c.Total*remainsWindowShare < window {
			return nil
		}
	} else if c.Total < remainsFloorTokens {
		return nil
	}
	return &c
}
