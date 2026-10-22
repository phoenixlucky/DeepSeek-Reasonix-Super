package boot

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

// sideScriptProvider answers the main session plainly. For the side conversation it
// first tries to write a file, as a model told to "just fix it" might, then
// answers once that was refused.
type sideScriptProvider struct {
	mu       sync.Mutex
	target   string
	sideReqs []provider.Request
}

func (p *sideScriptProvider) Name() string { return "boot-side" }

func (p *sideScriptProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk, 6)
	side := len(req.Messages) > 0 && strings.Contains(req.Messages[0].Content, "side question beside their main session")
	switch {
	case !side:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "main answer"}
	default:
		p.mu.Lock()
		p.sideReqs = append(p.sideReqs, req)
		p.mu.Unlock()
		attempted := false
		for _, m := range req.Messages {
			if m.Role == provider.RoleTool {
				attempted = true
			}
		}
		if !attempted {
			ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
				ID: "w1", Name: "write_file", Arguments: `{"path":` + jsonQuote(p.target) + `,"content":"changed by the side"}`,
			}}
		} else {
			ch <- provider.Chunk{Type: provider.ChunkText, Text: "side answer"}
		}
		ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 100, CompletionTokens: 7, TotalTokens: 107}}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func jsonQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// A side conversation, through the real Build: it answers, it cannot write, it
// leaves the main transcript and the workspace alone, it bills the session
// under its own source, and closing it leaves nothing running.
func TestEffectSideConversationIsReadOnlyEphemeralAndBilled(t *testing.T) {
	home := statedBootHome(t)
	dir := testenv.TempDir(t)
	target := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	prov := &sideScriptProvider{target: target}
	kind := "boot-side-" + t.Name()
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return prov, nil })
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
	approveWorkspaceAt(t, home, dir)

	var mu sync.Mutex
	var usage []event.Event
	var mainTexts []string
	sink := event.FuncSink(func(e event.Event) {
		mu.Lock()
		defer mu.Unlock()
		switch e.Kind {
		case event.Usage:
			usage = append(usage, e)
		case event.Text, event.Message:
			mainTexts = append(mainTexts, e.Text)
		}
	})
	ctrl, err := Build(context.Background(), Options{Home: home, WorkspaceRoot: dir, Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := ctrl.Run(context.Background(), "hello"); err != nil {
		t.Fatalf("main turn: %v", err)
	}
	historyBefore := len(ctrl.History())
	mu.Lock()
	textsBefore, usageBefore := len(mainTexts), len(usage)
	mu.Unlock()
	goroutinesBefore := runtime.NumGoroutine()

	id, err := ctrl.OpenSide()
	if err != nil {
		t.Fatalf("OpenSide: %v", err)
	}
	answer, err := ctrl.AskSide(context.Background(), id, "what is in victim.txt?", []string{"quoted passage"})
	if err != nil || answer != "side answer" {
		t.Fatalf("AskSide = %q %v", answer, err)
	}

	if got, _ := os.ReadFile(target); string(got) != "original" {
		t.Fatalf("the side wrote to the workspace: %q", got)
	}
	if len(ctrl.History()) != historyBefore {
		t.Fatalf("the main transcript went from %d to %d messages", historyBefore, len(ctrl.History()))
	}
	mu.Lock()
	if len(mainTexts) != textsBefore {
		t.Fatalf("the side's words reached the main stream: %v", mainTexts[textsBefore:])
	}
	side := append([]event.Event(nil), usage[usageBefore:]...)
	mu.Unlock()
	if len(side) == 0 {
		t.Fatal("what the side cost never reached the session")
	}
	for _, e := range side {
		if e.UsageSource != event.UsageSourceSide {
			t.Fatalf("a side bill is labelled %q", e.UsageSource)
		}
	}

	prov.mu.Lock()
	first := prov.sideReqs[0]
	prov.mu.Unlock()
	names := map[string]bool{}
	for _, tl := range first.Tools {
		names[tl.Name] = true
	}
	for _, writer := range []string{"write_file", "edit_file", "remember", "todo_write", "task"} {
		if names[writer] {
			t.Fatalf("the side was shown %q: %v", writer, names)
		}
	}
	if !names["read_file"] {
		t.Fatalf("the side cannot read: %v", names)
	}

	ctrl.CloseSide(id)
	if _, err := ctrl.AskSide(context.Background(), id, "again", nil); err == nil {
		t.Fatal("a closed side still answers")
	}
	ctrl.Close()
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > goroutinesBefore && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > goroutinesBefore {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines %d before, %d after closing:\n%s", goroutinesBefore, after, buf[:runtime.Stack(buf, true)])
	}
}
