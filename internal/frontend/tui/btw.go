package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/base/i18n"
)

// btwMsg is a side question's answer, or why there is none.
type btwMsg struct {
	answer string
	err    error
}

// Side asks one question in a side conversation and closes it again: the
// kernel keeps it in memory only and never in the main transcript.
func (c *Client) Side(ctx context.Context, question string) (string, error) {
	var opened struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/side", nil, &opened); err != nil {
		return "", err
	}
	defer func() {
		_ = c.do(context.WithoutCancel(ctx), http.MethodDelete, "/side/"+url.PathEscape(opened.ID), nil, nil)
	}()
	var out struct {
		Answer string `json:"answer"`
	}
	err := c.do(ctx, http.MethodPost, "/side/"+url.PathEscape(opened.ID)+"/ask", map[string]string{"question": question}, &out)
	return out.Answer, err
}

func (m *model) btwRun(question string) func() tea.Msg {
	question = strings.TrimSpace(m.pastes.expand(question))
	return func() tea.Msg {
		if question == "" {
			return slashDoneMsg{level: "info", text: i18n.M.BtwUsage}
		}
		answer, err := m.client.Side(m.ctx, question)
		return btwMsg{answer: answer, err: err}
	}
}

func (m *model) onBtw(msg btwMsg) tea.Cmd {
	if msg.err != nil {
		m.tr.AddNotice("warn", btwFailure(msg.err))
		return m.commit()
	}
	answer := strings.TrimSpace(msg.answer)
	if answer == "" {
		answer = i18n.M.BtwNoAnswer
	}
	m.tr.AddNotice("info", i18n.M.BtwSavedNote+"\n"+answer)
	return m.commit()
}

// btwFailure speaks from the kernel's refusal code, never from its sentence.
func btwFailure(err error) string {
	switch Code(err) {
	case "side.unavailable":
		return i18n.M.BtwUnavailable
	case "side.limit":
		return i18n.M.BtwLimit
	case "side.busy":
		return i18n.M.BtwBusy
	case "side.empty":
		return i18n.M.BtwUsage
	}
	return fmt.Sprintf(i18n.M.BtwFailedFmt, err.Error())
}
