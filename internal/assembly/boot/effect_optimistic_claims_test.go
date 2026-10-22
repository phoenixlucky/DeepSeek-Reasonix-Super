package boot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/runtime/writeclaim"
)

// slowChildProvider scripts a parent that starts an undeclared background writer
// and then writes a file of its own, while the child is held in its first model
// call so it is certainly still running when the parent writes.
type slowChildProvider struct {
	mu          sync.Mutex
	parent      int
	parentTools int
	target      string
	started     chan struct{}
	release     chan struct{}
	startedO    sync.Once
}

func (p *slowChildProvider) Name() string { return "boot-effect-slow-child" }

func (p *slowChildProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	// The first model request with tools is the parent's; the sub-agent's tool
	// surface is a different size, which tells the two apart.
	p.mu.Lock()
	if p.parentTools == 0 {
		p.parentTools = len(req.Tools)
	}
	isParent := len(req.Tools) == p.parentTools
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 3)
	if len(req.Tools) == 0 {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "title"}
		ch <- provider.Chunk{Type: provider.ChunkDone}
		close(ch)
		return ch, nil
	}
	if !isParent {
		p.startedO.Do(func() { close(p.started) })
		select {
		case <-p.release:
		case <-ctx.Done():
		}
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "child done"}
		ch <- provider.Chunk{Type: provider.ChunkDone}
		close(ch)
		return ch, nil
	}
	p.mu.Lock()
	p.parent++
	n := p.parent
	p.mu.Unlock()
	switch n {
	case 1:
		args, _ := json.Marshal(map[string]any{"prompt": "slow edits", "run_in_background": true, "description": "slow"})
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "t1", Name: "task", Arguments: string(args)}}
	case 2:
		select {
		case <-p.started:
		case <-time.After(20 * time.Second):
		}
		args, _ := json.Marshal(map[string]string{"path": p.target, "content": "from the parent\n"})
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "w1", Name: "write_file", Arguments: string(args)}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

var (
	slowChildCurrent    atomic.Pointer[slowChildProvider]
	slowChildRegistered sync.Once
)

func registerSlowChild() {
	provider.Register("boot-effect-slow-child", func(provider.Config) (provider.Provider, error) { return slowChildCurrent.Load(), nil })
}

func runParentWriteBesideChild(t *testing.T, claims string) (landed bool, result event.Tool) {
	t.Helper()
	home := isolateConfigHome(t)
	_ = os.MkdirAll(filepath.Join(home, ".reasonix", "sessions"), 0o755)
	dir := robustTempDir(t)
	t.Chdir(dir)
	prov := &slowChildProvider{target: filepath.Join(dir, "parent.txt"), started: make(chan struct{}), release: make(chan struct{})}
	slowChildCurrent.Store(prov)
	slowChildRegistered.Do(registerSlowChild)
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"
`)
	userConfig := config.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(userConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userConfig, []byte("[agent]\nsubagent_claims = \""+claims+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "boot-effect-slow-child"
model = "x"
`)
	approveWorkspace(t, dir)
	sink := &childReceiptSink{done: make(chan struct{})}
	ctrl, err := Build(context.Background(), Options{Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	_ = os.WriteFile(filepath.Join(home, ".reasonix", "sessions", "o.execution.jsonl"), nil, 0o600)
	ctrl.SetSessionPath(filepath.Join(dir, ".reasonix", "sessions", "o.jsonl"))
	ctrl.SetBypass(true)
	ctrl.Submit("go")
	select {
	case <-sink.done:
	case <-time.After(30 * time.Second):
		sink.mu.Lock()
		for _, e := range sink.events {
			t.Logf("event kind=%v tool=%s err=%q text=%.80q out=%.120q approval=%+v detail=%.100q", e.Kind, e.Tool.Name, e.Tool.Err, e.Text, e.Tool.Output, e.Approval, e.Detail)
		}
		sink.mu.Unlock()
		t.Fatal("turn did not finish")
	}
	close(prov.release)
	_, statErr := os.Stat(prov.target)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, e := range sink.events {
		if e.Kind == event.ToolResult && e.Tool.Name == "write_file" {
			result = e.Tool
		}
	}
	return statErr == nil, result
}

func TestEffectOptimisticClaimsLetTheParentWriteBesideAnUndeclaredChild(t *testing.T) {
	landed, res := runParentWriteBesideChild(t, "optimistic")
	if !landed || res.Err != "" {
		t.Fatalf("the parent's write beside an undeclared child: landed=%v err=%q output=%q", landed, res.Err, res.Output)
	}
}

func TestEffectStrictClaimsStillRefuseTheParentWriteWithATypedCode(t *testing.T) {
	landed, res := runParentWriteBesideChild(t, "strict")
	if landed {
		t.Fatal("under strict claims the parent wrote while an undeclared child held the workspace")
	}
	if res.RefusalCode != writeclaim.CodeWritePathBusy {
		t.Fatalf("refusal code = %q (err %q), want %q", res.RefusalCode, res.Err, writeclaim.CodeWritePathBusy)
	}
	if !strings.HasPrefix(res.Output, "blocked") {
		t.Fatalf("the model was not told why: %q", res.Output)
	}
}
