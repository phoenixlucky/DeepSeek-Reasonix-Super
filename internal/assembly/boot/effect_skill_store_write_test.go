package boot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/ext/skilllock"
)

type skillStoreWriter struct {
	mu    sync.Mutex
	calls int
	paths []string
}

func (p *skillStoreWriter) Name() string { return "boot-effect-skill-store-write" }

func (p *skillStoreWriter) Stream(_ context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 3)
	if n <= len(p.paths) {
		args, _ := json.Marshal(map[string]any{"path": p.paths[n-1], "content": "planted"})
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "w" + string(rune('0'+n)), Name: "write_file", Arguments: string(args)}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// The agent cannot write the installed skills, nor the record of where they came
// from: both sit in the Reasonix home, outside every folder a session may write.
func TestEffectTheAgentCannotWriteTheInstalledSkillsOrTheirSourceRecord(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	reasonixHome := config.ReasonixHomeDir()
	skillFile := filepath.Join(reasonixHome, "skills", "mine", "SKILL.md")
	lockFile := skilllock.Path(reasonixHome)
	prov := &skillStoreWriter{paths: []string{skillFile, lockFile}}
	provider.Register("boot-effect-skill-store-write", func(provider.Config) (provider.Provider, error) { return prov, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "boot-effect-skill-store-write"
model = "x"
`)
	approveWorkspace(t, dir)
	sink := &childReceiptSink{done: make(chan struct{})}
	ctrl, err := Build(context.Background(), Options{Sink: sink})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	_ = os.MkdirAll(filepath.Join(dir, ".reasonix", "sessions"), 0o755)
	ctrl.SetSessionPath(filepath.Join(dir, ".reasonix", "sessions", "p.jsonl"))
	ctrl.SetBypass(true)
	ctrl.Submit("improve my skill")
	select {
	case <-sink.done:
	case <-time.After(60 * time.Second):
		t.Fatal("turn did not finish")
	}
	for _, p := range []string{skillFile, lockFile} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("the agent wrote %s", p)
		}
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	refused := 0
	for _, e := range sink.events {
		if e.Kind == event.ToolResult && e.Tool.Name == "write_file" && e.Tool.Err != "" {
			refused++
		}
	}
	if refused != 2 {
		t.Fatalf("%d of the two writes were refused", refused)
	}
}
