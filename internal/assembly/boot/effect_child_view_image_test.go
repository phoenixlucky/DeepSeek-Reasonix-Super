package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
)

// seenRequests records every request the scripted providers receive, by which
// configured provider answered, so a test can say what the parent and the child
// were each offered.
type seenRequests struct {
	mu   sync.Mutex
	by   map[string][]provider.Request
	call int
}

func (s *seenRequests) add(label string, req provider.Request) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.by == nil {
		s.by = map[string][]provider.Request{}
	}
	s.by[label] = append(s.by[label], req)
	s.call++
	return len(s.by[label])
}

func (s *seenRequests) first(label string) (provider.Request, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.by[label]) == 0 {
		return provider.Request{}, false
	}
	return s.by[label][0], true
}

type scriptedLabelProvider struct {
	label, childRef string
	seen            *seenRequests
}

func (p *scriptedLabelProvider) Name() string { return "boot-effect-child-image-" + p.label }

func (p *scriptedLabelProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	n := p.seen.add(p.label, req)
	ch := make(chan provider.Chunk, 3)
	if p.label == "main" && n == 1 && p.childRef != "" {
		args, _ := json.Marshal(map[string]any{"prompt": "describe the screenshot", "model": p.childRef})
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "t1", Name: "task", Arguments: string(args)}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func childImageRun(t *testing.T, kind, childRef string) *seenRequests {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	seen := &seenRequests{}
	provider.Register(kind, func(cfg provider.Config) (provider.Provider, error) {
		return &scriptedLabelProvider{label: map[string]string{"x": "main", "y": "seer", "z": "plain"}[cfg.Model], childRef: childRef, seen: seen}, nil
	})
	writeFile(t, dir, "reasonix.toml", fmt.Sprintf(`
default_model = "test-model"

[agent]
system_prompt = "BASE"

[[providers]]
name = "test-model"
kind = %[1]q
model = "x"

[[providers]]
name = "seer"
kind = %[1]q
model = "y"
vision = true

[[providers]]
name = "plain"
kind = %[1]q
model = "z"
`, kind))
	approveWorkspace(t, dir)
	sink := &childReceiptSink{done: make(chan struct{})}
	ctrl, err := Build(context.Background(), Options{Sink: sink})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ctrl.Close()
	if ctrl, err = Build(context.Background(), Options{Sink: sink}); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	defer func() { ctrl.Close() }()
	if err := os.MkdirAll(config.SessionDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	ctrl.SetSessionPath(filepath.Join(config.SessionDir(), kind+".jsonl"))
	ctrl.SetBypass(true)
	ctrl.Submit("look at the screenshot")
	select {
	case <-sink.done:
	case <-time.After(90 * time.Second):
		t.Fatal("turn did not finish")
	}
	return seen
}

func offered(req provider.Request, name string) bool {
	for _, tl := range req.Tools {
		if tl.Name == name {
			return true
		}
	}
	return false
}

// A text-only session that delegates to a model reading images gives that child
// an image reader; the parent's own request, and a child on a text-only model,
// are unchanged.
func TestEffectChildOnAnImageModelIsOfferedViewImageAndTheParentIsNot(t *testing.T) {
	var control, withSeer, withPlain *seenRequests
	t.Run("control", func(t *testing.T) { control = childImageRun(t, "boot-effect-child-image-control", "") })
	t.Run("image child", func(t *testing.T) { withSeer = childImageRun(t, "boot-effect-child-image-seer", "seer/y") })
	t.Run("text child", func(t *testing.T) { withPlain = childImageRun(t, "boot-effect-child-image-plain", "plain/z") })

	child, ok := withSeer.first("seer")
	if !ok {
		for label, reqs := range withSeer.by {
			for i, r := range reqs {
				t.Logf("%s #%d last message: %.300s", label, i+1, r.Messages[len(r.Messages)-1].Content)
			}
		}
		t.Fatal("the delegated child never reached its provider")
	}
	if !offered(child, "view_image") {
		t.Fatalf("the child on an image model was not offered view_image; it saw %v", toolSchemaNames(child.Tools))
	}
	if plain, ok := withPlain.first("plain"); !ok {
		t.Fatal("the text-only child never reached its provider")
	} else if offered(plain, "view_image") {
		t.Fatal("a child on a text-only model was offered view_image")
	}

	base, _ := control.first("main")
	for name, run := range map[string]*seenRequests{"image child": withSeer, "text child": withPlain} {
		parent, _ := run.first("main")
		if offered(parent, "view_image") {
			t.Fatalf("%s: the text-only parent was offered view_image", name)
		}
		if !reflect.DeepEqual(parent.Tools, base.Tools) || !reflect.DeepEqual(parent.Messages[:1], base.Messages[:1]) {
			t.Fatalf("%s: the parent's tool block or system prompt changed because it delegated to another model", name)
		}
	}
}
