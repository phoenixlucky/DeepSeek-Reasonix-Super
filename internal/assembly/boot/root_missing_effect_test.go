package boot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
)

// goneRootProvider asks for one tool per round, then ends the turn.
type goneRootProvider struct {
	mu    sync.Mutex
	calls []provider.ToolCall
	turn  int
	reqs  []provider.Request
}

func (p *goneRootProvider) Name() string { return "boot-root-missing" }

func (p *goneRootProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	i := p.turn
	p.turn++
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 3)
	if i < len(p.calls) {
		call := p.calls[i]
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &call}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

type refusalSink struct {
	mu    sync.Mutex
	codes []string
}

func (s *refusalSink) Emit(e event.Event) {
	if e.Kind == event.ToolResult && e.Tool.RefusalCode != "" {
		s.mu.Lock()
		s.codes = append(s.codes, e.Tool.RefusalCode)
		s.mu.Unlock()
	}
}

func goneCall(id, name string, args map[string]any) provider.ToolCall {
	raw, _ := json.Marshal(args)
	return provider.ToolCall{ID: id, Name: name, Arguments: string(raw)}
}

// TestEffectGoneProjectFolderReachesTheModelAsATypedCause pins the boundary the
// component tests cannot: with the project folder moved away mid-session, the
// provider sees the typed cause for every tool that needs the folder — including
// glob and grep, which used to answer "no matches" — and the frontend sink gets
// the same code.
func TestEffectGoneProjectFolderReachesTheModelAsATypedCause(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)

	rec := &goneRootProvider{calls: []provider.ToolCall{
		goneCall("c1", "ls", map[string]any{}),
		goneCall("c2", "glob", map[string]any{"pattern": "*.go"}),
		goneCall("c3", "grep", map[string]any{"pattern": "hello"}),
		goneCall("c4", "read_file", map[string]any{"path": "a.txt"}),
	}}
	provider.Register("boot-root-missing", func(provider.Config) (provider.Provider, error) { return rec, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[codegraph]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-root-missing"
model = "x"
`)
	approveWorkspace(t, dir)

	sink := &refusalSink{}
	ctrl, err := Build(context.Background(), Options{Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	gone := dir + "-moved"
	// A directory a process is working in cannot be moved on Windows.
	t.Chdir(filepath.Dir(dir))
	if err := os.Rename(dir, gone); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows holds a folder the host has open and will not move it mid-session: %v", err)
		}
		t.Fatalf("move the project folder away: %v", err)
	}
	t.Cleanup(func() { _ = os.Rename(gone, dir) })
	if err := ctrl.Run(context.Background(), "look around"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rec.mu.Lock()
	last := rec.reqs[len(rec.reqs)-1]
	rec.mu.Unlock()
	results := effectToolResults(last)
	if len(results) != 4 {
		t.Fatalf("want one result per call at the provider boundary, got %d: %q", len(results), results)
	}
	for i, result := range results {
		if !strings.HasPrefix(result, "root missing: the project folder ") || !strings.Contains(result, "not a permission or argument problem") {
			t.Errorf("result %d reached the model as %q, want the typed cause", i, effectHead(result))
		}
		if strings.Contains(result, "no matches") || strings.Contains(result, "The system cannot find") || strings.Contains(result, "no such file") {
			t.Errorf("result %d still carries a bare system answer: %q", i, effectHead(result))
		}
		if !strings.Contains(result, filepath.Base(dir)) {
			t.Errorf("result %d does not name the folder: %q", i, effectHead(result))
		}
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.codes) != 4 {
		t.Fatalf("frontend sink saw refusal codes %v, want 4", sink.codes)
	}
	for _, code := range sink.codes {
		if code != tool.CodeRootMissing {
			t.Errorf("sink code = %q, want %q", code, tool.CodeRootMissing)
		}
	}
}
