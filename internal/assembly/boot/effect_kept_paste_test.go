package boot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/session/control"
)

// keptPasteProvider reads the pasted file back the way a model told to would:
// one windowed read_file, then an answer. It records every request it is sent.
type keptPasteProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
	path string
}

func (p *keptPasteProvider) Name() string { return "boot-kept-paste" }

func (p *keptPasteProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 4)
	read := false
	for _, m := range req.Messages {
		if m.Role == provider.RoleTool {
			read = true
		}
	}
	if p.path != "" && !read {
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{
			ID: "r1", Name: "read_file", Arguments: fmt.Sprintf(`{"path":%s,"offset":40,"limit":3}`, jsonQuote(p.path)),
		}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "read it"}
	}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 100, CompletionTokens: 5, TotalTokens: 105}}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

// waitFor blocks until at least n requests have arrived: Submit answers on the
// turn's own goroutine, as it does for every frontend.
func (p *keptPasteProvider) waitFor(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got := p.texts(); len(got) >= n {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("only %d provider requests arrived, want %d", len(p.texts()), n)
	return nil
}

func (p *keptPasteProvider) texts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, r := range p.reqs {
		var b strings.Builder
		for _, m := range r.Messages {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
		out = append(out, b.String())
	}
	return out
}

// A paste a person kept as a file leaves only a pointer in what the provider is
// sent; the model can still read the file by window; and the "@path" form, which
// the turn parser expands into the file's head, would have put the text back.
func TestEffectKeptPasteReachesTheProviderAsAPointerOnly(t *testing.T) {
	home := statedBootHome(t)
	dir := testenv.TempDir(t)
	var lines []string
	for i := range 600 {
		lines = append(lines, fmt.Sprintf("LOG-%04d something noisy happened here", i))
	}
	pasted := strings.Join(lines, "\n") + "\n"

	prov := &keptPasteProvider{}
	kind := "boot-kept-paste-" + t.Name()
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
	done := make(chan struct{}, 4)
	sink := event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnDone {
			done <- struct{}{}
		}
	})
	ctrl, err := Build(context.Background(), Options{Home: home, WorkspaceRoot: dir, Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()

	rel, err := control.SaveTextAttachmentInRoot(ctrl.WorkspaceRoot(), pasted)
	if err != nil {
		t.Fatal(err)
	}
	prov.path = rel
	pointer := control.TextAttachmentPointer(rel, pasted)
	ctrl.Submit("what went wrong here?\n\n" + pointer)
	reqs := prov.waitFor(t, 2)
	<-done
	if first := reqs[0]; !strings.Contains(first, rel) || strings.Contains(first, "LOG-0007") || strings.Contains(first, "LOG-0599") {
		t.Fatalf("the first request is not the pointer alone:\n%.600s", first)
	}
	if second := reqs[1]; !strings.Contains(second, "LOG-0040") || strings.Contains(second, "LOG-0300") {
		t.Fatalf("read_file did not return just the requested window:\n%.900s", second)
	}

	// The form the paste must not take: an "@" reference is expanded by the
	// turn parser into the head of the file.
	prov.path = ""
	before := len(prov.texts())
	ctrl.Submit("again: @" + rel)
	if got := prov.waitFor(t, before+1)[before]; !strings.Contains(got, "LOG-0007") {
		t.Fatalf("the @ form no longer puts the file's head in the message, so the premise for sending a pointer instead has changed:\n%.600s", got)
	}
}
