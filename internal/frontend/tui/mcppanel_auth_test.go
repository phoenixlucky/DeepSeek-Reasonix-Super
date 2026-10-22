package tui

import (
	"strings"
	"testing"
)

func authRows() []map[string]any {
	return []map[string]any{
		{"name": "figma", "state": "failed", "enabled": true, "transport": "http", "httpStatus": 401},
		{"name": "notes", "state": "ready", "enabled": true, "transport": "http", "tools": 1, "scopeUpgrade": []string{"files:write", "admin"}},
		{"name": "docs", "state": "ready", "enabled": true, "transport": "http", "tools": 2},
		{"name": "down", "state": "failed", "enabled": true, "transport": "http", "httpStatus": 500},
	}
}

func TestMCPPanelSignsInOnlyAServerThatAnswered401(t *testing.T) {
	m, k := testModel(t)
	k.mcpRows = authRows()
	enterMCPLine(m, "/mcp")
	if v := m.View().Content; !strings.Contains(v, "press a") {
		t.Fatalf("no sign-in hint for a 401 row:\n%s", v)
	}
	run(m, keyRune(m, 'a'))
	if calls := strings.Join(k.seen(), "\n"); !strings.Contains(calls, `POST /mcp/signin {"name":"figma"}`) {
		t.Fatalf("calls:\n%s", calls)
	}
	for _, sel := range []int{2, 3} {
		m.mcp.sel = sel
		before := len(k.seen())
		run(m, keyRune(m, 'a'))
		if len(k.seen()) != before {
			t.Fatalf("a signed in a server (row %d) that did not answer 401", sel)
		}
	}
}

func TestMCPPanelUpgradeShowsTheServersScopesAndAsksOnlyForAServerThatAsked(t *testing.T) {
	m, k := testModel(t)
	k.mcpRows = authRows()
	enterMCPLine(m, "/mcp")
	m.mcp.sel = 1
	if v := m.View().Content; !strings.Contains(v, "files:write admin") || !strings.Contains(v, "press u") {
		t.Fatalf("the scope words and the key are not shown:\n%s", v)
	}
	run(m, keyRune(m, 'u'))
	if calls := strings.Join(k.seen(), "\n"); !strings.Contains(calls, `POST /mcp/upgrade {"name":"notes"}`) {
		t.Fatalf("calls:\n%s", calls)
	}
	m.mcp.sel = 2
	before := len(k.seen())
	run(m, keyRune(m, 'u'))
	if len(k.seen()) != before {
		t.Fatal("u upgraded a server that asked for nothing")
	}
}
