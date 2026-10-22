package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/base/i18n"
	"reasonix/internal/frontend/termrender"
)

// queueRowLimit is how many queued messages are drawn under the working line;
// the rest are named by a count and listed by /queue.
const queueRowLimit = 3

// queueView is the durable queue as the screen holds it: the kernel's snapshot,
// and the one message the arrow keys have selected into the composer.
type queueView struct {
	items  []InboxItem
	paused bool
	// selID names the selected message; empty when none is.
	selID string
	// draft is what the composer held when the selection began, set aside and
	// put back when it ends.
	draft string
	// loaded is the selected message's full text as put in the composer, to tell a
	// key that acts on the message from one that edits it. ready says it has been
	// read: nothing is saved over a message whose text is not in the composer.
	loaded string
	ready  bool
	// confirm is a first d waiting for its second.
	confirm bool
}

type queueSnapMsg struct {
	snap InboxSnapshot
	err  error
}

type queueBodyMsg struct {
	id   string
	body string
	err  error
}

// queueDoneMsg settles a key's change to the queue: a note, and a fresh read.
type queueDoneMsg struct {
	level, text string
	err         error
	// deleted is the id of a message this key removed from the queue.
	deleted string
}

func (m *model) fetchQueue() tea.Cmd {
	ctx, c := m.ctx, m.client
	return func() tea.Msg {
		snap, err := c.Inbox(ctx)
		return queueSnapMsg{snap: snap, err: err}
	}
}

func (m *model) onQueueRowsMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case queueSnapMsg:
		if msg.err != nil {
			return nil, true
		}
		m.queue.items, m.queue.paused = msg.snap.Items, msg.snap.Paused
		if m.queue.selID != "" && m.queueIndex() < 0 {
			// The message ran or was removed elsewhere: the edit ends and the
			// draft comes back, so nothing is saved over or sent again.
			id := m.queue.selID
			m.queueEnd(true)
			m.tr.AddNotice("warn", fmt.Sprintf(i18n.M.QueueGoneFmt, shortID(id)))
			return m.commit(), true
		}
		return nil, true
	case queueBodyMsg:
		if msg.id != m.queue.selID {
			return nil, true
		}
		if msg.err != nil {
			m.queueEnd(true)
			m.tr.AddNotice("warn", fmt.Sprintf(i18n.M.QueueActionFailedFmt, msg.err))
			return m.commit(), true
		}
		m.queue.loaded, m.queue.ready = msg.body, true
		m.composer.SetValue(msg.body)
		return nil, true
	case queueDoneMsg:
		level := msg.level
		text := msg.text
		if msg.err != nil {
			level, text = "warn", fmt.Sprintf(i18n.M.QueueActionFailedFmt, msg.err)
		}
		m.tr.DropQueued(msg.deleted)
		m.tr.AddNotice(level, text)
		return tea.Batch(m.commit(), m.fetchQueue()), true
	}
	return nil, false
}

// queueIndex is where the selected message sits in the snapshot, -1 when none
// is selected or it has left the queue.
func (m *model) queueIndex() int {
	for i, it := range m.queue.items {
		if it.ID == m.queue.selID {
			return i
		}
	}
	return -1
}

// queueTag is what the working line says about the queue.
func (m *model) queueTag() string {
	if len(m.queue.items) == 0 {
		return ""
	}
	return " · " + fmt.Sprintf(i18n.M.QueueTagFmt, len(m.queue.items))
}

// queueRows draws the queued messages above the composer: up to queueRowLimit
// numbered lines with the selected one marked, then a count of the rest.
func (m *model) queueRows() []string {
	if len(m.queue.items) == 0 {
		return nil
	}
	width := m.contentWidth()
	sel := m.queueIndex()
	start := 0
	if sel >= queueRowLimit {
		start = sel - queueRowLimit + 1
	}
	var out []string
	for i := start; i < len(m.queue.items) && i < start+queueRowLimit; i++ {
		line := fmt.Sprintf("[%d] %s", i+1, oneLine(m.queue.items[i].Preview, width-10))
		if i == sel {
			out = append(out, "  "+termrender.Accent("▸ "+line))
			continue
		}
		out = append(out, termrender.Dim("    "+line))
	}
	if more := len(m.queue.items) - start - queueRowLimit; more > 0 {
		out = append(out, termrender.Dim("    "+fmt.Sprintf(i18n.M.QueueRowsMoreFmt, more)))
	}
	if m.queue.paused {
		out = append(out, termrender.Dim("  "+i18n.M.QueueRowsPaused))
	}
	return out
}

// queued reports whether a pending row of the transcript is one the queue rows
// already draw, so a message is not drawn twice.
func (m *model) queued(it *Item) bool {
	if it.QueueID == "" {
		return false
	}
	for _, q := range m.queue.items {
		if q.ID == it.QueueID {
			return true
		}
	}
	return false
}

// queueKey takes the keys that browse and manage the queue: up and down select
// a queued message into the composer, alt+up and alt+down move it, d (twice)
// deletes it, r runs a blocked one again, enter saves what was edited, esc lets
// go, and space pauses or resumes the queue when the composer is empty.
func (m *model) queueKey(k string) (tea.Cmd, bool) {
	selected := m.queue.selID != ""
	acting := selected && m.queue.ready && m.composer.Value() == m.queue.loaded
	switch {
	case k == "up" && len(m.queue.items) > 0 && (m.tr.Running || m.queue.paused) && (selected || m.composer.Line() == 0):
		return m.queueSelect(-1), true
	case k == "down" && selected:
		return m.queueSelect(1), true
	case (k == "alt+up" || k == "alt+down") && selected:
		return m.queueMove(k == "alt+up"), true
	case k == "esc" && selected:
		m.queueEnd(true)
		return nil, true
	case k == "enter" && selected:
		if !m.queue.ready {
			return nil, true
		}
		return m.queueSave(), true
	case k == "d" && acting:
		return m.queueDelete(), true
	case k == "r" && acting:
		return m.queueRetry(), true
	case (k == "space" || k == " ") && len(m.queue.items) > 0 && m.composer.Value() == "":
		return m.queuePause(), true
	}
	if selected && k != "alt+up" && k != "alt+down" {
		m.queue.confirm = false
	}
	return nil, false
}

// queueSelect moves the selection by delta and puts the message in the
// composer; past the newest it ends the selection.
func (m *model) queueSelect(delta int) tea.Cmd {
	at := m.queueIndex()
	switch m.queue.selID {
	case "":
		m.queue.draft = m.composer.Value()
		at = len(m.queue.items) - 1
	default:
		at += delta
	}
	if at < 0 {
		at = 0
	}
	if at >= len(m.queue.items) {
		m.queueEnd(true)
		return nil
	}
	m.queue.confirm = false
	m.queue.selID = m.queue.items[at].ID
	m.queue.loaded, m.queue.ready = "", false
	id, ctx, c := m.queue.selID, m.ctx, m.client
	return func() tea.Msg {
		body, err := c.InboxBody(ctx, id)
		return queueBodyMsg{id: id, body: body, err: err}
	}
}

// queueEnd lets go of the selection; the draft comes back when asked.
func (m *model) queueEnd(restore bool) {
	if restore {
		m.composer.SetValue(m.queue.draft)
	}
	m.queue.selID, m.queue.loaded, m.queue.draft, m.queue.confirm, m.queue.ready = "", "", "", false, false
}

func (m *model) queueMove(up bool) tea.Cmd {
	at := m.queueIndex()
	to := at + 1
	if up {
		to = at - 1
	}
	if at < 0 || to < 0 || to >= len(m.queue.items) {
		return nil
	}
	id, ctx, c := m.queue.selID, m.ctx, m.client
	return func() tea.Msg {
		if err := c.InboxMove(ctx, id, to); err != nil {
			return queueDoneMsg{err: err}
		}
		snap, err := c.Inbox(ctx)
		return queueSnapMsg{snap: snap, err: err}
	}
}

func (m *model) queueSave() tea.Cmd {
	text, id, ctx, c := strings.TrimSpace(m.composer.Value()), m.queue.selID, m.ctx, m.client
	if text == "" {
		return nil
	}
	m.queueEnd(true)
	return func() tea.Msg {
		if err := c.InboxUpdate(ctx, id, text); err != nil {
			return queueDoneMsg{err: err}
		}
		return queueDoneMsg{level: "info", text: fmt.Sprintf(i18n.M.QueueSavedFmt, shortID(id))}
	}
}

func (m *model) queueDelete() tea.Cmd {
	id, ctx, c := m.queue.selID, m.ctx, m.client
	if !m.queue.confirm {
		m.queue.confirm = true
		m.tr.AddNotice("info", fmt.Sprintf(i18n.M.QueueDeleteAgainFmt, shortID(id)))
		return m.commit()
	}
	m.queueEnd(true)
	return func() tea.Msg {
		if err := c.InboxDelete(ctx, id); err != nil {
			return queueDoneMsg{err: err}
		}
		return queueDoneMsg{level: "info", text: fmt.Sprintf(i18n.M.QueueDeletedFmt, shortID(id)), deleted: id}
	}
}

func (m *model) queueRetry() tea.Cmd {
	id, ctx, c := m.queue.selID, m.ctx, m.client
	m.queueEnd(true)
	return func() tea.Msg {
		if err := c.InboxRetry(ctx, id); err != nil {
			return queueDoneMsg{err: err}
		}
		return queueDoneMsg{level: "info", text: fmt.Sprintf(i18n.M.QueueRetriedFmt, shortID(id))}
	}
}

func (m *model) queuePause() tea.Cmd {
	paused, ctx, c := !m.queue.paused, m.ctx, m.client
	return func() tea.Msg {
		if err := c.InboxPause(ctx, paused); err != nil {
			return queueDoneMsg{err: err}
		}
		note := i18n.M.QueueResumedNote
		if paused {
			note = i18n.M.QueuePausedNote
		}
		return queueDoneMsg{level: "info", text: note}
	}
}
