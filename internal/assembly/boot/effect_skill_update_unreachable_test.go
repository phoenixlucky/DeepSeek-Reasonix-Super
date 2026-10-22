package boot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/provider"
)

type toolListProvider struct {
	mu    sync.Mutex
	tools []provider.ToolSchema
}

func (p *toolListProvider) Name() string { return "boot-effect-tool-list" }

func (p *toolListProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	if p.tools == nil {
		p.tools = req.Tools
	}
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// Updating an installed skill from its source folder is the person's act. Even
// in the most permissive mode the model's tool table holds nothing that reaches
// it: not a tool of its own, and no tool that mentions the update routes.
func TestEffectNoToolTheModelHoldsReachesTheSkillUpdate(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	prov := &toolListProvider{}
	provider.Register("boot-effect-tool-list", func(provider.Config) (provider.Provider, error) { return prov, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "boot-effect-tool-list"
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
	ctrl.Submit("hi")
	select {
	case <-sink.done:
	case <-time.After(60 * time.Second):
		t.Fatal("turn did not finish")
	}
	prov.mu.Lock()
	tools := prov.tools
	prov.mu.Unlock()
	if len(tools) == 0 {
		t.Fatal("the provider saw no tools; the assertion would hold for nothing")
	}
	for _, tl := range tools {
		raw, _ := json.Marshal(tl)
		text := strings.ToLower(string(raw))
		if strings.Contains(tl.Name, "skill_update") || strings.Contains(text, "update/apply") || strings.Contains(text, "update/plan") || strings.Contains(text, "skillupdate") {
			t.Errorf("tool %q reaches the skill update: %s", tl.Name, raw)
		}
	}
}
