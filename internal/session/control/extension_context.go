package control

import (
	"fmt"
	"strings"
	"unicode"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/extension/dispatch"
)

// frameExtensionContext is the turn-tail text for the context blocks extensions
// added: each under the host's external-content header naming the plugin that
// added it, cut to the user's ceilings. The second result says what the ceilings
// cut, in the user's words. The cache-stable prefix is never touched.
func frameExtensionContext(d *dispatch.Dispatcher, p dispatch.InputPayload) (string, string) {
	limits := config.LoadForEdit(config.UserConfigPath()).ModsLimits()
	blocks, from := p.Context, p.ContextFrom
	left := 0
	if limits.ContextBlocks > 0 && len(blocks) > limits.ContextBlocks {
		left = len(blocks) - limits.ContextBlocks
		blocks = blocks[:limits.ContextBlocks]
	}
	var parts []string
	cut := 0
	for i, b := range blocks {
		kind := tool.ProvenanceExtension
		plugin := ""
		if i < len(from) {
			plugin = from[i]
		}
		if d.IsMod(plugin) {
			kind = tool.ProvenanceMod
		}
		text := b.Text
		if limits.ContextBytes > 0 && len(text) > limits.ContextBytes {
			rest := len(text) - limits.ContextBytes
			text = strings.ToValidUTF8(text[:limits.ContextBytes], "") + fmt.Sprintf("\n[truncated: %d bytes omitted by the context_bytes ceiling]", rest)
			cut++
		}
		parts = append(parts, strings.TrimSuffix(tool.ProvenanceHeader(tool.Provenance{Kind: kind, Source: plugin}), "\n")+"\n"+labelLine(b.Label)+text)
	}
	if left > 0 {
		parts = append(parts, fmt.Sprintf("[%d more context blocks omitted by the context_blocks ceiling]", left))
	}
	note := ""
	if cut > 0 || left > 0 {
		note = fmt.Sprintf("Context an extension added was cut to your ceilings: %d block(s) shortened, %d left out.", cut, left)
	}
	return strings.Join(parts, "\n\n"), note
}

// labelLine reduces a label to printable words so it cannot write into the
// header above it.
func labelLine(label string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(label) {
		if b.Len() >= 48 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || strings.ContainsRune("._-", r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "label: " + b.String() + "\n"
}
