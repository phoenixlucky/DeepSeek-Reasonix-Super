package tui

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/base/i18n"
	"reasonix/internal/base/textutil"
	"reasonix/internal/frontend/termrender"
)

// MCPTool is one tool a server offers.
type MCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"readOnly"`
	Destructive bool   `json:"destructive"`
}

// MCPServer is one configured or connected MCP server as the kernel reports it.
type MCPServer struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	Enabled     bool      `json:"enabled"`
	Transport   string    `json:"transport"`
	Source      string    `json:"source"`
	Description string    `json:"description"`
	Tools       int       `json:"tools"`
	ToolList    []MCPTool `json:"toolList"`
	Error       string    `json:"error"`
	Launch      string    `json:"launch"`
	// PendingReason is the kernel's code for why a pending server waits.
	PendingReason string `json:"pendingReason"`
	// HTTPStatus is what the endpoint answered when the failure was an HTTP one.
	HTTPStatus int `json:"httpStatus"`
	// ScopeUpgrade is the access a signed-in server asked for, in its own words.
	ScopeUpgrade []string `json:"scopeUpgrade"`
}

func (c *Client) MCPServers(ctx context.Context) ([]MCPServer, error) {
	var out struct {
		Servers []MCPServer `json:"servers"`
	}
	err := c.do(ctx, http.MethodGet, "/mcp", nil, &out)
	return out.Servers, err
}

func (c *Client) SetMCPEnabled(ctx context.Context, name string, enabled bool) error {
	return c.do(ctx, http.MethodPost, "/mcp/enabled",
		map[string]any{"name": name, "enabled": enabled, "scope": "project"}, nil)
}

func (c *Client) ReconnectMCP(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/mcp/reconnect", map[string]string{"name": name}, nil)
}

// SignInMCP runs the browser sign-in for one server and returns when it ends.
func (c *Client) SignInMCP(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/mcp/signin", map[string]string{"name": name}, nil)
}

// UpgradeMCP grants a signed-in server the access it asked for.
func (c *Client) UpgradeMCP(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/mcp/upgrade", map[string]string{"name": name}, nil)
}

// mcpPanel lists the MCP servers; Enter on one shows its tools.
type mcpPanel struct {
	servers []MCPServer
	sel     int
	detail  bool
	busy    bool
	confirm string
	// input, review and removeAsk are the add and remove flows in progress.
	input     *mcpInput
	review    *mcpReview
	removeAsk string
}

type mcpMsg struct {
	servers []MCPServer
	err     error
	keep    string
	open    bool
}

func (m *model) openMCP() tea.Cmd { return m.fetchMCP("", false) }

func (m *model) fetchMCP(keep string, open bool) tea.Cmd {
	return func() tea.Msg {
		list, err := m.client.MCPServers(m.ctx)
		return mcpMsg{servers: list, err: err, keep: keep, open: open}
	}
}

func (m *model) onMCP(msg mcpMsg) tea.Cmd {
	if msg.err != nil {
		m.tr.AddNotice("error", fmt.Sprintf(i18n.M.McpPanelErrFmt, msg.err))
		if m.mcp != nil {
			m.mcp.busy = false
		}
		return m.commit()
	}
	p := m.mcp
	if p == nil {
		p = &mcpPanel{}
		m.mcp = p
	}
	p.servers, p.busy = msg.servers, false
	for i, s := range p.servers {
		if s.Name == msg.keep {
			p.sel = i
		}
	}
	p.sel = max(min(p.sel, len(p.servers)-1), 0)
	return nil
}

func (m *model) mcpKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	p := m.mcp
	if p == nil {
		return nil, false
	}
	if p.busy {
		return nil, true
	}
	if p.input != nil {
		return m.mcpInputKey(msg)
	}
	if p.review != nil {
		return m.mcpReviewKey(msg)
	}
	if len(p.servers) == 0 {
		switch msg.String() {
		case "n":
			p.input = &mcpInput{}
		case "q", "esc", "ctrl+c", "left", "h":
			m.mcp = nil
		}
		return nil, true
	}
	cur := p.servers[p.sel]
	if p.removeAsk != "" {
		name := p.removeAsk
		p.removeAsk = ""
		if msg.String() == "y" && name == cur.Name {
			return m.mcpRemove(name), true
		}
		return nil, true
	}
	if p.confirm != "" {
		if msg.String() == "y" && p.confirm == cur.Name {
			p.confirm = ""
			return m.mcpToggle(cur), true
		}
		p.confirm = ""
		return nil, true
	}
	return m.mcpListKey(msg, cur)
}

// mcpListKey takes the keys of the server list itself.
func (m *model) mcpListKey(msg tea.KeyPressMsg, cur MCPServer) (tea.Cmd, bool) {
	p := m.mcp
	switch msg.String() {
	case "q":
		m.mcp = nil
	case "esc", "ctrl+c", "left", "h":
		if p.detail {
			p.detail = false
		} else {
			m.mcp = nil
		}
	case "up", "k":
		p.sel = max(p.sel-1, 0)
	case "down", "j":
		p.sel = min(p.sel+1, len(p.servers)-1)
	case "enter", "right", "l":
		p.detail = true
	case "n":
		p.input = &mcpInput{}
	case "d":
		p.removeAsk = cur.Name
	case "space":
		if !cur.Enabled && repoDeclared(cur) {
			p.confirm = cur.Name
			return nil, true
		}
		return m.mcpToggle(cur), true
	case "a":
		if !cur.Enabled || cur.State != "failed" || cur.HTTPStatus != http.StatusUnauthorized {
			return nil, true
		}
		p.busy = true
		return m.mcpAction(cur.Name, func(ctx context.Context) error { return m.client.SignInMCP(ctx, cur.Name) }), true
	case "u":
		if !cur.Enabled || len(cur.ScopeUpgrade) == 0 {
			return nil, true
		}
		p.busy = true
		return m.mcpAction(cur.Name, func(ctx context.Context) error { return m.client.UpgradeMCP(ctx, cur.Name) }), true
	case "r":
		if !cur.Enabled || cur.State != "failed" {
			return nil, true
		}
		p.busy = true
		return m.mcpAction(cur.Name, func(ctx context.Context) error {
			return m.client.ReconnectMCP(ctx, cur.Name)
		}), true
	}
	return nil, true
}

// repoDeclared reports a server a repository's own files declare: starting
// it runs what a stranger wrote.
func repoDeclared(s MCPServer) bool {
	return s.Source == "project_mcp_json" || s.Source == "project_config"
}

func (m *model) mcpToggle(s MCPServer) tea.Cmd {
	m.mcp.busy = true
	return m.mcpAction(s.Name, func(ctx context.Context) error {
		return m.client.SetMCPEnabled(ctx, s.Name, !s.Enabled)
	})
}

func (m *model) mcpAction(name string, do func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		if err := do(m.ctx); err != nil {
			return mcpActionErrMsg{name: name, err: err}
		}
		list, err := m.client.MCPServers(m.ctx)
		return mcpMsg{servers: list, err: err, keep: name}
	}
}

type mcpActionErrMsg struct {
	name string
	err  error
}

func (m *model) onMCPActionErr(msg mcpActionErrMsg) tea.Cmd {
	m.tr.AddNotice("error", fmt.Sprintf(i18n.M.McpActionErrFmt, msg.name, msg.err))
	return tea.Batch(m.commit(), m.fetchMCP(msg.name, false))
}

func (m *model) mcpPanelLines() []string {
	p := m.mcp
	switch {
	case p.input != nil:
		return m.mcpInputLines()
	case p.review != nil:
		return m.mcpReviewLines()
	case len(p.servers) == 0:
		lines := []string{termrender.Accent(i18n.M.McpPanelTitle)}
		for part := range strings.SplitSeq(i18n.M.McpEmptyHelp, " — ") {
			lines = append(lines, termrender.Dim("  "+clipVisible(part, max(m.width-8, 12))))
		}
		return panel(append(lines, termrender.Dim(i18n.M.McpEmptyHint)), m.width, accentEdge)
	}
	cur := p.servers[p.sel]
	if p.detail {
		return m.mcpDetail(cur)
	}
	on := 0
	for _, s := range p.servers {
		if s.Enabled {
			on++
		}
	}
	lines := []string{termrender.Accent(i18n.M.McpPanelTitle),
		termrender.Dim("  " + fmt.Sprintf(i18n.M.McpPanelSummaryFmt, len(p.servers), on))}
	start := max(min(p.sel-pickerRows/2, len(p.servers)-pickerRows), 0)
	end := min(start+pickerRows, len(p.servers))
	if start > 0 {
		lines = append(lines, termrender.Dim("  "+i18n.M.ListMoreAbove))
	}
	width := max(m.width-8, 12)
	for i := start; i < end; i++ {
		s := p.servers[i]
		state := s.State
		switch {
		case s.State == "pending" && s.PendingReason == "changed_since_enabled":
			state = i18n.M.McpPanelChanged
		case s.State == "pending":
			state = i18n.M.McpPanelPending
		case !s.Enabled:
			state = i18n.M.McpPanelOff
		}
		meta := []string{state}
		if s.Tools > 0 {
			meta = append(meta, fmt.Sprintf(i18n.M.McpPanelToolsFmt, s.Tools))
		}
		if s.Transport != "" {
			meta = append(meta, s.Transport)
		}
		if s.Source != "" {
			meta = append(meta, s.Source)
		}
		lines = append(lines, rowLine(i == p.sel, i+1, "", clipVisible(textutil.SanitizeLaunch(s.Name)+" · "+strings.Join(meta, " · "), width), s.Enabled))
	}
	if end < len(p.servers) {
		lines = append(lines, termrender.Dim("  "+i18n.M.ListMoreBelow))
	}
	if p.removeAsk != "" {
		lines = append(lines, termrender.Yellow("  "+clipVisible(fmt.Sprintf(i18n.M.McpRemoveAskFmt, textutil.SanitizeLaunch(p.removeAsk)), width)))
	} else if p.confirm != "" {
		lines = append(lines, termrender.Yellow("  "+fmt.Sprintf(i18n.M.McpPanelConfirmFmt, textutil.SanitizeLaunch(p.confirm), clipVisible(textutil.SanitizeLaunch(cur.Launch), width-8))))
	} else {
		if cur.State == "pending" && cur.Launch != "" {
			lines = append(lines, termrender.Yellow("  "+fmt.Sprintf(i18n.M.McpPanelLaunchFmt, clipVisible(textutil.SanitizeLaunch(cur.Launch), width-8))))
		}
		lines = append(lines, mcpAuthHint(cur, width)...)
		lines = append(lines, termrender.Dim(i18n.M.McpPanelHint))
	}
	return panel(lines, m.width, accentEdge)
}

// mcpAuthHint names the one key that answers the server under the cursor, when
// one does. The scope names are the server's words and are shown as written.
func mcpAuthHint(s MCPServer, width int) []string {
	switch {
	case s.Enabled && len(s.ScopeUpgrade) > 0:
		return []string{termrender.Yellow("  " + clipVisible(fmt.Sprintf(i18n.M.McpPanelUpgradeFmt, textutil.SanitizeLaunch(strings.Join(s.ScopeUpgrade, " "))), width))}
	case s.Enabled && s.State == "failed" && s.HTTPStatus == http.StatusUnauthorized:
		return []string{termrender.Yellow("  " + i18n.M.McpPanelSignIn)}
	}
	return nil
}

func (m *model) mcpDetail(s MCPServer) []string {
	width := max(m.width-8, 12)
	lines := []string{termrender.Accent(textutil.SanitizeLaunch(s.Name)), termrender.Dim("  " + textutil.SanitizeLaunch(s.State+" · "+s.Transport+" · "+s.Source))}
	if s.Description != "" {
		lines = append(lines, "  "+clipVisible(strings.Join(strings.Fields(s.Description), " "), width))
	}
	if s.Error != "" {
		lines = append(lines, "  "+termrender.Red(clipVisible(strings.Join(strings.Fields(s.Error), " "), width)))
	}
	if len(s.ToolList) == 0 {
		lines = append(lines, termrender.Dim("  "+i18n.M.McpPanelNoTools))
	}
	for i, t := range s.ToolList {
		if i >= pickerRows*2 {
			lines = append(lines, termrender.Dim("  "+fmt.Sprintf(i18n.M.ListMoreFmt, len(s.ToolList)-i)))
			break
		}
		tag := ""
		switch {
		case t.Destructive:
			tag = " · " + i18n.M.McpToolDestructive
		case t.ReadOnly:
			tag = " · " + i18n.M.McpToolReadOnly
		}
		lines = append(lines, "  "+clipVisible(t.Name+tag, width))
	}
	lines = append(lines, termrender.Dim(i18n.M.McpPanelDetailHint))
	return panel(lines, m.width, accentEdge)
}
