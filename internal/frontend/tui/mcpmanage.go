package tui

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/base/i18n"
	"reasonix/internal/base/textutil"
	"reasonix/internal/frontend/termrender"
)

// mcpDraftServer is one server a paste resolved to. The display fields are what
// the person is shown; the asIs ones go back to the kernel unchanged on install.
type mcpDraftServer struct {
	Name           string            `json:"name"`
	Transport      string            `json:"transport"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	URL            string            `json:"url,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	DisplayURL     string            `json:"displayUrl,omitempty"`
	DisplayEnv     map[string]string `json:"displayEnv,omitempty"`
	DisplayHeaders map[string]string `json:"displayHeaders,omitempty"`
	DisplayCommand string            `json:"displayCommand,omitempty"`
	DisplayArgs    []string          `json:"displayArgs,omitempty"`
}

type mcpDraftRisk struct {
	Server string `json:"server"`
	Kind   string `json:"kind"`
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

type mcpDraft struct {
	Servers []mcpDraftServer `json:"servers"`
	Risks   []mcpDraftRisk   `json:"risks"`
}

// MCPInstallResult is what installing one server came to; State says whether it
// is usable (ready), needs the person (action_required) or failed (issue).
type MCPInstallResult struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	ToolCount int    `json:"toolCount"`
	Action    string `json:"action"`
	Message   string `json:"message"`
}

// ParseMCP resolves a pasted command, address or JSON without installing anything.
func (c *Client) ParseMCP(ctx context.Context, input string) (mcpDraft, error) {
	var out mcpDraft
	err := c.do(ctx, http.MethodPost, "/mcp/parse", map[string]string{"input": input}, &out)
	return out, err
}

func (c *Client) InstallMCP(ctx context.Context, s mcpDraftServer, scope string) (MCPInstallResult, error) {
	var out MCPInstallResult
	err := c.do(ctx, http.MethodPost, "/mcp/install", map[string]any{"server": s, "scope": scope}, &out)
	return out, err
}

type mcpRemoved struct {
	Name            string `json:"name"`
	StillConfigured bool   `json:"stillConfigured"`
}

func (c *Client) RemoveMCP(ctx context.Context, name string) (mcpRemoved, error) {
	var out mcpRemoved
	err := c.do(ctx, http.MethodPost, "/mcp/remove", map[string]string{"name": name}, &out)
	return out, err
}

// mcpInput is the line a server is described on; mcpReview is what that line
// would start, shown whole before anything is installed.
type mcpInput struct{ text string }

type mcpReview struct {
	draft   mcpDraft
	armedAt time.Time
}

// mcpReviewDelay is how long a review stays deaf to y, l and p after it
// appears, so a key already on its way cannot approve what was not yet read.
var mcpReviewDelay = 400 * time.Millisecond

// mcpReviewLimit is the most the review will show whole; past it nothing can be
// installed from the screen, because a person cannot be asked to approve a
// command they could not read to the end.
const mcpReviewLimit = 4096

type mcpParsedMsg struct {
	draft mcpDraft
	err   error
}

type mcpInstalledMsg struct {
	results []MCPInstallResult
	err     error
}

type mcpRemovedMsg struct {
	removed mcpRemoved
	err     error
}

// mcpInputKey takes the keys while a server is being described.
func (m *model) mcpInputKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	p := m.mcp
	switch k := msg.String(); k {
	case "esc", "ctrl+c":
		p.input = nil
	case "enter":
		text := strings.TrimSpace(p.input.text)
		if text == "" {
			return nil, true
		}
		p.busy = true
		return func() tea.Msg {
			draft, err := m.client.ParseMCP(m.ctx, text)
			return mcpParsedMsg{draft: draft, err: err}
		}, true
	case "backspace":
		if t := p.input.text; t != "" {
			_, n := utf8.DecodeLastRuneInString(t)
			p.input.text = t[:len(t)-n]
		}
	default:
		if t := typedText(msg); t != "" && !hasControl(t) {
			p.input.text += t
		}
	}
	return nil, true
}

// mcpPaste adds pasted text to the line being typed; false when none is.
func (m *model) mcpPaste(text string) bool {
	if m.mcp == nil || m.mcp.input == nil || m.mcp.busy {
		return false
	}
	m.mcp.input.text += strings.Join(strings.Fields(text), " ")
	return true
}

func (m *model) onMCPParsed(msg mcpParsedMsg) tea.Cmd {
	p := m.mcp
	if p == nil {
		return nil
	}
	p.busy = false
	if msg.err != nil || len(msg.draft.Servers) == 0 {
		p.input = nil
		reason := "no server found"
		if msg.err != nil {
			reason = msg.err.Error()
		}
		m.tr.AddNotice("error", fmt.Sprintf(i18n.M.McpParseFailedFmt, reason))
		return m.commit()
	}
	p.input = nil
	p.review = &mcpReview{draft: msg.draft, armedAt: time.Now().Add(mcpReviewDelay)}
	return nil
}

// mcpReviewKey takes the keys while a draft is under review. Nothing is
// installed except by y, l or p pressed here, so no turn and no model can start
// a server through the panel.
func (m *model) mcpReviewKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	p := m.mcp
	scope := map[string]string{"y": "user", "l": "local", "p": "project"}[msg.String()]
	if scope == "" {
		if k := msg.String(); k == "n" || k == "esc" || k == "ctrl+c" || k == "q" {
			p.review = nil
		}
		return nil, true
	}
	if p.review.tooLong() || time.Now().Before(p.review.armedAt) {
		return nil, true
	}
	draft := p.review.draft
	p.review, p.busy = nil, true
	return func() tea.Msg {
		var out mcpInstalledMsg
		for _, s := range draft.Servers {
			res, err := m.client.InstallMCP(m.ctx, s, scope)
			if err != nil {
				out.err = fmt.Errorf("%s: %w", s.Name, err)
				break
			}
			out.results = append(out.results, res)
		}
		return out
	}, true
}

func (m *model) onMCPInstalled(msg mcpInstalledMsg) tea.Cmd {
	keep := ""
	for _, r := range msg.results {
		keep = r.Name
		if r.State == "ready" {
			m.tr.AddNotice("info", fmt.Sprintf(i18n.M.McpAddInstalledFmt, textutil.SanitizeLaunch(r.Name), r.ToolCount))
		} else {
			m.tr.AddNotice("warn", fmt.Sprintf(i18n.M.McpAddNeedsFmt, textutil.SanitizeLaunch(r.Name), textutil.SanitizeLaunch(strings.Join(strings.Fields(r.Message), " "))))
		}
	}
	if msg.err != nil {
		m.tr.AddNotice("error", fmt.Sprintf(i18n.M.McpAddFailedFmt, msg.err))
	}
	return tea.Batch(m.commit(), m.fetchMCP(keep, false))
}

func (m *model) onMCPRemoved(msg mcpRemovedMsg) tea.Cmd {
	if msg.err != nil {
		m.tr.AddNotice("error", fmt.Sprintf(i18n.M.McpAddFailedFmt, msg.err))
		return tea.Batch(m.commit(), m.fetchMCP("", false))
	}
	name := textutil.SanitizeLaunch(msg.removed.Name)
	if msg.removed.StillConfigured {
		m.tr.AddNotice("warn", fmt.Sprintf(i18n.M.McpRemovedStillFmt, name))
	} else {
		m.tr.AddNotice("info", fmt.Sprintf(i18n.M.McpRemovedFmt, name))
	}
	return tea.Batch(m.commit(), m.fetchMCP("", false))
}

func (m *model) mcpRemove(name string) tea.Cmd {
	m.mcp.busy = true
	return func() tea.Msg {
		got, err := m.client.RemoveMCP(m.ctx, name)
		return mcpRemovedMsg{removed: got, err: err}
	}
}

// tooLong says the draft is more than the review will show whole.
func (r *mcpReview) tooLong() bool {
	n := 0
	for _, s := range r.draft.Servers {
		n += len(s.DisplayCommand) + len(s.DisplayURL) + len(s.Name)
		for _, a := range s.DisplayArgs {
			n += len(a) + 1
		}
		for _, mp := range []map[string]string{s.DisplayEnv, s.DisplayHeaders} {
			for k, v := range mp {
				n += len(k) + len(v) + 2
			}
		}
	}
	return n > mcpReviewLimit
}

// wrapRows breaks one logical line into display rows of at most width columns,
// the first after prefix and the rest indented, so nothing is ever cut off.
func wrapRows(prefix, text string, width int, style func(string) string) []string {
	indent := strings.Repeat(" ", termrender.Cells().StringWidth(prefix))
	inner := max(width-termrender.Cells().StringWidth(prefix), 8)
	var rows []string
	for i, row := range strings.Split(termrender.Hardwrap(text, inner), "\n") {
		lead := indent
		if i == 0 {
			lead = prefix
		}
		rows = append(rows, style(lead+row))
	}
	return rows
}

func asIs(s string) string { return s }

func (m *model) mcpInputLines() []string {
	width := max(m.width-4, 12)
	lines := []string{termrender.Accent(i18n.M.McpPanelTitle)}
	lines = append(lines, wrapRows("  ", i18n.M.McpAddPrompt, width, termrender.Dim)...)
	typed := wrapRows("  ", textutil.SanitizeLaunch(m.mcp.input.text)+"▏", width, asIs)
	const keep = 8
	if len(typed) > keep {
		typed = append([]string{termrender.Dim("  …")}, typed[len(typed)-keep:]...)
	}
	lines = append(lines, typed...)
	lines = append(lines, wrapRows("  ", i18n.M.McpAddEchoNote, width, termrender.Dim)...)
	return panel(lines, m.width, accentEdge)
}

// mcpReviewLines shows each server whole, wrapped rather than cut: the command
// or address with secrets redacted by the kernel, the environment and headers,
// and every warning the kernel found. A draft too long to show whole is not
// offered for install at all.
func (m *model) mcpReviewLines() []string {
	width := max(m.width-4, 12)
	var lines []string
	for _, s := range m.mcp.review.draft.Servers {
		lines = append(lines, wrapRows("", fmt.Sprintf(i18n.M.McpReviewTitleFmt, textutil.SanitizeLaunch(s.Name)), width, termrender.Accent)...)
		switch {
		case s.URL != "":
			lines = append(lines, wrapRows("  ", fmt.Sprintf(i18n.M.McpReviewConnectsFmt, textutil.SanitizeLaunch(s.DisplayURL)), width, asIs)...)
		default:
			line := strings.Join(append([]string{s.DisplayCommand}, s.DisplayArgs...), " ")
			lines = append(lines, wrapRows("  ", fmt.Sprintf(i18n.M.McpReviewRunsFmt, textutil.SanitizeLaunch(line)), width, asIs)...)
		}
		if env := joinPairs(s.DisplayEnv, s.DisplayHeaders); env != "" {
			lines = append(lines, wrapRows("  ", fmt.Sprintf(i18n.M.McpReviewEnvFmt, textutil.SanitizeLaunch(env)), width, termrender.Dim)...)
		}
	}
	if len(m.mcp.review.draft.Risks) > 0 {
		lines = append(lines, termrender.Yellow("  "+i18n.M.McpReviewRisksHead))
		for _, r := range m.mcp.review.draft.Risks {
			lines = append(lines, wrapRows("  - ", textutil.SanitizeLaunch(r.Detail), width, termrender.Yellow)...)
		}
	}
	if m.mcp.review.tooLong() {
		return panel(append(lines, wrapRows("", i18n.M.McpReviewTooLong, width, termrender.Red)...), m.width, accentEdge)
	}
	for _, row := range wrapParts(i18n.M.McpReviewKeys, " · ", width) {
		lines = append(lines, termrender.Dim(row))
	}
	return panel(lines, m.width, accentEdge)
}

func joinPairs(maps ...map[string]string) string {
	var parts []string
	for _, mp := range maps {
		for k, v := range mp {
			parts = append(parts, k+"="+v)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// onMCPMsg takes the answers to what the MCP panel asked for.
func (m *model) onMCPMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case mcpMsg:
		return m.onMCP(msg), true
	case mcpActionErrMsg:
		return m.onMCPActionErr(msg), true
	case mcpParsedMsg:
		return m.onMCPParsed(msg), true
	case mcpInstalledMsg:
		return m.onMCPInstalled(msg), true
	case mcpRemovedMsg:
		return m.onMCPRemoved(msg), true
	}
	return nil, false
}

// wrapParts breaks a " · "-separated hint into rows no wider than width,
// never inside a part.
func wrapParts(text, sep string, width int) []string {
	var rows []string
	cur := ""
	for part := range strings.SplitSeq(text, sep) {
		switch {
		case cur == "":
			cur = part
		case termrender.Cells().StringWidth(cur+sep+part) <= width:
			cur += sep + part
		default:
			rows = append(rows, cur)
			cur = part
		}
	}
	if cur != "" {
		rows = append(rows, cur)
	}
	return rows
}
