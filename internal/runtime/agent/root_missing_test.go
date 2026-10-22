package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/sessionstore"
)

// rootedTool names the directories its call acts under, as a built-in does.
type rootedTool struct {
	name  string
	roots []tool.Root
	calls atomic.Int32
}

func (t *rootedTool) Name() string            { return t.name }
func (t *rootedTool) Description() string     { return t.name }
func (t *rootedTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *rootedTool) ReadOnly() bool          { return true }
func (t *rootedTool) Execute(context.Context, json.RawMessage) (string, error) {
	t.calls.Add(1)
	return "(no matches)", nil
}
func (t *rootedTool) RootsFor(json.RawMessage) []tool.Root { return t.roots }

func rootTestAgent(t *testing.T, tools ...tool.Tool) *Agent {
	t.Helper()
	reg := tool.NewRegistry()
	for _, candidate := range tools {
		reg.Add(candidate)
	}
	return New(nil, reg, sessionstore.NewSession(""), Options{CheckTargetAccess: allowTestFileAccess}, event.Discard)
}

func TestCallWhoseDirectoryIsGoneIsRefusedBeforeItRunsWithTheTypedCause(t *testing.T) {
	base := testenv.TempDir(t)
	gone := filepath.Join(base, "project")
	cases := []struct {
		name  string
		root  tool.Root
		shows string
		hides string
	}{
		{"workspace", tool.Root{Kind: tool.RootWorkspace, Path: gone, Display: gone}, gone, ""},
		{"external", tool.Root{Kind: tool.RootExternal, Path: gone, Display: "__reasonix_external_folder/abc/meta"}, "__reasonix_external_folder/abc/meta", gone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := &rootedTool{name: "glob", roots: []tool.Root{tc.root}}
			a := rootTestAgent(t, probe)
			outcome := a.executeOne(context.Background(), &a.turn, provider.ToolCall{ID: "c1", Name: "glob", Arguments: `{}`})
			if probe.calls.Load() != 0 {
				t.Fatal("the tool ran although its directory does not exist, so an empty result could say 'no matches'")
			}
			if !outcome.blocked || outcome.refusalCode != tool.CodeRootMissing {
				t.Fatalf("outcome = %+v, want a blocked call carrying %s", outcome, tool.CodeRootMissing)
			}
			if !strings.Contains(outcome.output, tc.shows) || !strings.Contains(outcome.output, "not a permission or argument problem") {
				t.Fatalf("the model must be told which directory and what kind of failure: %s", outcome.output)
			}
			if tc.hides != "" && strings.Contains(outcome.output, tc.hides) {
				t.Fatalf("the real path of an external folder reached the model: %s", outcome.output)
			}
		})
	}
}

func TestCallWhoseDirectoryExistsRunsAsBefore(t *testing.T) {
	dir := testenv.TempDir(t)
	probe := &rootedTool{name: "glob", roots: []tool.Root{{Kind: tool.RootWorkspace, Path: dir, Display: dir}}}
	a := rootTestAgent(t, probe)
	outcome := a.executeOne(context.Background(), &a.turn, provider.ToolCall{ID: "c1", Name: "glob", Arguments: `{}`})
	if probe.calls.Load() != 1 || outcome.blocked || outcome.refusalCode != "" {
		t.Fatalf("a call whose directory exists must run untouched: calls=%d outcome=%+v", probe.calls.Load(), outcome)
	}
}

func TestCallWithNoDirectoryIsNotChecked(t *testing.T) {
	probe := &rootedTool{name: "glob"}
	a := rootTestAgent(t, probe)
	a.executeOne(context.Background(), &a.turn, provider.ToolCall{ID: "c1", Name: "glob", Arguments: `{}`})
	if probe.calls.Load() != 1 {
		t.Fatal("a tool that names no directory must run as before")
	}
}

func TestDirectoryThatReturnsLetsTheNextCallRun(t *testing.T) {
	dir := filepath.Join(testenv.TempDir(t), "project")
	probe := &rootedTool{name: "glob", roots: []tool.Root{{Kind: tool.RootWorkspace, Path: dir, Display: dir}}}
	a := rootTestAgent(t, probe)
	first := a.executeOne(context.Background(), &a.turn, provider.ToolCall{ID: "c1", Name: "glob", Arguments: `{}`})
	if !first.blocked {
		t.Fatalf("first call should be refused: %+v", first)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	second := a.executeOne(context.Background(), &a.turn, provider.ToolCall{ID: "c2", Name: "glob", Arguments: `{}`})
	if second.blocked || probe.calls.Load() != 1 {
		t.Fatalf("the check is per call, not remembered: %+v calls=%d", second, probe.calls.Load())
	}
}
