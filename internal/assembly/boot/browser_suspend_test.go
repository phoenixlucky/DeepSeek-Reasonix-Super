package boot

import (
	"testing"

	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/builtin"
)

// browserToolNames lists the browser tool names present in a registry.
func browserToolNames(reg *tool.Registry) []string {
	want := map[string]bool{}
	for _, t := range builtin.BrowserTools(nil) {
		want[t.Name()] = true
	}
	var out []string
	for _, name := range reg.Names() {
		if want[name] {
			out = append(out, name)
		}
	}
	return out
}

// Dropping the browser tools has to be permanent, not just an absent binding:
// `addBuiltins` seeds every built-in before `bindBrowser` runs, and a later
// `Add` — an MCP handshake, a preset binding its own set — would put the unbound
// copies straight back. The names are suspended, which takes them out and keeps
// them out, and this pins the keeping-out half.
func TestSuspendedBrowserToolsStayOut(t *testing.T) {
	reg := tool.NewRegistry()
	for _, t := range builtin.BrowserTools(nil) {
		reg.Add(t)
	}
	if got := browserToolNames(reg); len(got) != 3 {
		t.Fatalf("browser tools seeded = %v, want all three in the registry first", got)
	}

	suspendBrowserTools(reg)
	if got := browserToolNames(reg); len(got) != 0 {
		t.Fatalf("suspended browser tools still offered: %v", got)
	}

	for _, t := range builtin.BrowserTools(nil) {
		reg.Add(t)
	}
	if got := browserToolNames(reg); len(got) != 0 {
		t.Fatalf("a later Add put the browser tools back: %v", got)
	}
}
