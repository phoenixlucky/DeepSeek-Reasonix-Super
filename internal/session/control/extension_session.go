package control

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"reasonix/internal/base/secrets"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/extension/protocol"
	"reasonix/internal/state/sessioninbox"
)

// Typed identities of the refusals an extension's session calls can get.
var (
	// ErrExtensionSubmitRate: the extension queued its share of prompts this turn.
	// ErrExtensionSubmitTooLong: one prompt over the user's submit_bytes ceiling.
	ErrExtensionSubmitTooLong = errors.New("an extension prompt is longer than the submit_bytes ceiling allows")
	ErrExtensionSubmitRate    = errors.New("extensions have queued as many prompts since you last wrote as the submit_per_turn ceiling allows")
	// ErrExtensionReadKind: a read of something the host does not serve.
	ErrExtensionReadKind = errors.New("the host serves only usage and messages")
)

// defaultExtensionMessages is how many transcript items a read returns when the
// extension names no limit.
const defaultExtensionMessages = 20

// extensionPrompts counts the prompts extensions queued since the user last
// spoke. A queued prompt opens a host turn, and that turn's hooks may queue
// another, so a count that reset at every turn would never stop the chain; the
// user's next own words are what end it.
type extensionPrompts struct {
	mu sync.Mutex
	n  int
}

func (p *extensionPrompts) take(ceiling int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ceiling > 0 && p.n >= ceiling {
		return false
	}
	p.n++
	return true
}

// userSpoke clears the count when a turn is the user's own words.
func (p *extensionPrompts) userSpoke(synthetic bool) {
	if synthetic {
		return
	}
	p.mu.Lock()
	p.n = 0
	p.mu.Unlock()
}

// QueueExtensionPrompt queues a prompt an extension asked for. It is the host's
// message about the extension, not the user's own words: queued with the host's
// origin and the extension as its source, so no gate that asks "did the user
// write this" can take it for yes.
func (c *Controller) QueueExtensionPrompt(plugin, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("an extension prompt must have text")
	}
	limits := config.LoadForEdit(config.UserConfigPath()).ModsLimits()
	if limits.SubmitBytes > 0 && len(text) > limits.SubmitBytes {
		return ErrExtensionSubmitTooLong
	}
	if !c.extPrompts.take(limits.SubmitPerTurn) {
		return ErrExtensionSubmitRate
	}
	source := tool.Provenance{Kind: tool.ProvenanceExtension, Source: plugin}
	if c.loadExtensions().IsMod(plugin) {
		source.Kind = tool.ProvenanceMod
	}
	label := string(source.Kind) + ":" + plugin
	display := fmt.Sprintf("[from %s] %s", label, text)
	_, err := c.EnqueueInbox(InboxRequest{
		Intent: sessioninbox.IntentFollowup, Origin: sessioninbox.OriginHost,
		Source:  "extension:" + plugin,
		Display: display,
		Submit:  tool.ProvenanceHeader(source) + text,
	})
	if err != nil {
		return err
	}
	if !c.Running() {
		c.maybeDispatchInbox()
	}
	return nil
}

// ReadExtensionSession answers an extension's read-only look at the session.
// Messages are credential-redacted and cut to the user's ceilings; usage is the
// last response's token counts.
func (c *Controller) ReadExtensionSession(p protocol.SessionReadParams) (protocol.SessionReadResult, error) {
	switch p.What {
	case protocol.SessionReadUsage:
		u := protocol.SessionUsage{}
		if last := c.LastUsage(); last != nil {
			u = protocol.SessionUsage{PromptTokens: last.PromptTokens, CompletionTokens: last.CompletionTokens, CachedTokens: last.CacheHitTokens}
		}
		return protocol.SessionReadResult{Usage: &u}, nil
	case protocol.SessionReadMessages:
		return c.readExtensionMessages(p.Limit), nil
	}
	return protocol.SessionReadResult{}, ErrExtensionReadKind
}

func (c *Controller) readExtensionMessages(limit int) protocol.SessionReadResult {
	limits := config.LoadForEdit(config.UserConfigPath()).ModsLimits()
	if limit <= 0 {
		limit = defaultExtensionMessages
	}
	if limits.MessagesMax > 0 {
		limit = min(limit, limits.MessagesMax)
	}
	var shown []provider.Message
	for _, m := range c.History() {
		if m.Role != provider.RoleSystem {
			shown = append(shown, m)
		}
	}
	omitted := max(len(shown)-limit, 0)
	shown = shown[omitted:]
	out := protocol.SessionReadResult{Omitted: omitted}
	for _, m := range shown {
		text := secrets.RedactCredentials(m.Content)
		if limits.ContextBytes > 0 && len(text) > limits.ContextBytes {
			text = strings.ToValidUTF8(text[:limits.ContextBytes], "") + "…"
		}
		out.Messages = append(out.Messages, protocol.SessionMessage{Role: string(m.Role), Text: text})
	}
	return out
}
