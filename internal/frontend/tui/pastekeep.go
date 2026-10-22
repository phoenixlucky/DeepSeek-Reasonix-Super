package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/base/i18n"
)

// pasteKeptMsg is the kernel's answer for the folded pastes asked to be kept:
// pointers[i] belongs to blocks[i], and err stops the list where it failed.
type pasteKeptMsg struct {
	blocks   []int
	pointers []string
	err      error
}

// KeepText stores a paste as a file under the workspace's attachments and
// returns the pointer a message carries in place of the text.
func (c *Client) KeepText(ctx context.Context, text string) (string, error) {
	var out struct {
		Pointer string `json:"pointer"`
	}
	err := c.do(ctx, http.MethodPost, "/attachments", map[string]string{"kind": "text", "text": text}, &out)
	return out.Pointer, err
}

// keepFoldedPastes saves every folded paste still in the draft as a file. It
// acts only on this key; a paste nobody asked to keep is sent whole.
func (m *model) keepFoldedPastes() tea.Cmd {
	draft := m.composer.Value()
	var idx []int
	for i, b := range m.pastes.blocks {
		if b.pointer == "" && strings.Contains(draft, b.label) {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return m.showFlash(i18n.M.PasteKeepNone)
	}
	texts := make([]string, len(idx))
	for n, i := range idx {
		texts[n] = m.pastes.blocks[i].text
	}
	return func() tea.Msg {
		out := pasteKeptMsg{blocks: idx}
		for _, text := range texts {
			pointer, err := m.client.KeepText(m.ctx, text)
			if err != nil {
				out.err = err
				break
			}
			out.pointers = append(out.pointers, pointer)
		}
		return out
	}
}

func (m *model) onPasteKept(msg pasteKeptMsg) tea.Cmd {
	draft := m.composer.Value()
	for n, pointer := range msg.pointers {
		b := &m.pastes.blocks[msg.blocks[n]]
		label := foldedFileLabel(b.label)
		draft = strings.ReplaceAll(draft, b.label, label)
		b.label, b.pointer = label, pointer
	}
	if len(msg.pointers) > 0 {
		m.composer.SetValue(draft)
	}
	if msg.err != nil {
		m.tr.AddNotice("error", fmt.Sprintf(i18n.M.PasteKeepFailedFmt, msg.err))
		return m.commit()
	}
	return m.showFlash(fmt.Sprintf(i18n.M.PasteKeptFmt, len(msg.pointers)))
}

// foldedFileLabel renames a folded text label to the kept form, so the draft
// shows which pastes will go as files.
func foldedFileLabel(label string) string {
	return strings.Replace(label, "[Pasted text #", "[Pasted file #", 1)
}
