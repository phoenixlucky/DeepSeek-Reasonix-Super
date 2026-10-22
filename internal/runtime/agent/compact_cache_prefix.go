package agent

import (
	"reflect"
	"slices"
	"strings"

	"reasonix/internal/contract/provider"
)

func (a *contextWindow) rememberSummaryPrefix(req provider.Request) {
	// Sampling already froze this request; retaining it must not copy a large
	// conversation a second time on every model round.
	a.sess.win.summaryPrefix.Store(&req)
}

func (a *contextWindow) withSummaryPrefix(fallback provider.Request, fold []provider.Message, instruction string) (provider.Request, bool) {
	held := a.sess.win.summaryPrefix.Load()
	if held == nil || len(fold) == 0 || a.effectiveContextWindow() <= 0 {
		return fallback, false
	}
	end := summaryPrefixEnd(held.Messages, summaryModelMessages(fold))
	if end <= 0 {
		return fallback, false
	}
	current := a.providerProjectionMessages(summaryModelMessages(a.modelVisibleMessages()))
	if end > len(current) || !slices.EqualFunc(held.Messages[:end], current[:end], sameSummaryMessage) {
		return fallback, false
	}
	prefix := held.Messages[:end]
	if closedPrefixEnd(prefix) != len(prefix) {
		return fallback, false
	}
	tail := instruction + summaryClosingInstruction
	if !summaryTextOnly(prefix) {
		end = 0
		for end < len(prefix) && prefix[end].Role == provider.RoleSystem {
			end++
		}
		if end == 0 {
			return fallback, false
		}
		prefix = prefix[:end]
		tail = instruction + "\n\n" + renderTranscript(fold) + summaryClosingInstruction
	}
	req := fallback
	req.Messages = append(slices.Clone(prefix), provider.Message{Role: provider.RoleUser, Content: tail})
	req.Tools = held.Tools
	req.Mode = held.Mode
	budget := min(summaryInputCeilingTokens, a.effectiveContextWindow()-req.MaxTokens-protocolReserveTokens)
	if a.estimatedRequestTokens(req) > budget {
		return fallback, false
	}
	if _, clipped, err := a.effectiveOutputBudget(req); err != nil || clipped {
		return fallback, false
	}
	return req, true
}

// A sparse fold can omit user turns kept verbatim. Both scans must agree on
// its endpoint, or repeated content cannot identify the prefix to summarize.
func summaryPrefixEnd(messages, fold []provider.Message) int {
	if len(fold) == 0 {
		return 0
	}
	end, next := 0, 0
	for i, message := range messages {
		if sameSummaryMessage(message, fold[next]) {
			next++
			if next == len(fold) {
				end = i + 1
				break
			}
		}
	}
	if end == 0 {
		return 0
	}
	next = len(fold) - 1
	last := 0
	for i, message := range slices.Backward(messages) {
		if sameSummaryMessage(message, fold[next]) {
			if next == len(fold)-1 {
				last = i + 1
			}
			next--
			if next < 0 {
				break
			}
		}
	}
	if last != end {
		return 0
	}
	return end
}

func summaryModelMessages(messages []provider.Message) []provider.Message {
	messages = slices.Clone(provider.ModelMessages(messages))
	for i := range messages {
		messages[i].CreatedAt = 0
	}
	return messages
}

func sameSummaryMessage(a, b provider.Message) bool { return reflect.DeepEqual(a, b) }

// Rich history must retain the argument and reasoning elision of renderTranscript.
func summaryTextOnly(messages []provider.Message) bool {
	for _, m := range messages {
		if len(m.ToolCalls) > 0 || len(m.Images) > 0 || len(m.ResponsesItems) > 0 || m.ReasoningContent != "" || m.ReasoningID != "" || m.ReasoningSignature != "" {
			return false
		}
	}
	return true
}

func (a *contextWindow) summaryRequest(region []provider.Message, instructions string, maxOut int, allowPrefix bool) (provider.Request, bool) {
	sys := summarySystemPrompt
	if strings.TrimSpace(instructions) != "" {
		sys += "\n\nAdditional focus for this compaction (prioritize keeping this):\n" + strings.TrimSpace(instructions)
	}
	req := provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: sys},
			{Role: provider.RoleUser, Content: renderTranscript(region) + summaryClosingInstruction},
		},
		MaxTokens:      maxOut,
		EffortOverride: summaryEffort,
		Summary:        true,
		Temperature:    provider.OptionalTemperature(a.temperature),
	}
	if allowPrefix {
		return a.withSummaryPrefix(req, region, sys)
	}
	return req, false
}
