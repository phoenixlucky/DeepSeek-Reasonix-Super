package boot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

// namedImageProvider is one configured model of the scenario. The text-only
// parent delegates once, naming the attachment in the task text, and says
// nothing about it otherwise; each model keeps what it was sent.
type namedImageProvider struct {
	label string
	call  func() provider.ToolCall
	seen  *seenRequests
}

func (p *namedImageProvider) Name() string { return "boot-effect-named-image-" + p.label }

func (p *namedImageProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	n := p.seen.add(p.label, req)
	ch := make(chan provider.Chunk, 3)
	if p.label == "main" && n == 1 {
		tc := p.call()
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &tc}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func imageCount(req provider.Request) int {
	n := 0
	for _, m := range req.Messages {
		n += len(m.Images)
	}
	return n
}

func lastUserText(req provider.Request) string {
	for _, m := range slices.Backward(req.Messages) {
		if m.Role == provider.RoleUser {
			return m.Content
		}
	}
	return ""
}

// delegateNamingAnAttachment runs a text-only parent that delegates with a task
// text pointing at a pasted image the user's own message never referenced (a
// follow-up turn), and returns what each configured model was sent.
func delegateNamingAnAttachment(t *testing.T, kind string, skill, viaTurn bool) *seenRequests {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	attachments := filepath.Join(dir, ".reasonix", "attachments")
	if err := os.MkdirAll(attachments, 0o700); err != nil {
		t.Fatal(err)
	}
	var shot bytes.Buffer
	if err := png.Encode(&shot, image.NewRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachments, "clipboard-pasted.png"), shot.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	seen := &seenRequests{}
	call := func() provider.ToolCall {
		task := "Describe .reasonix/attachments/clipboard-pasted.png for me."
		if viaTurn {
			task = "Describe the screenshot for me."
		}
		if skill {
			args, _ := json.Marshal(map[string]any{"name": "looker", "arguments": task})
			return provider.ToolCall{ID: "t1", Name: "read_only_skill", Arguments: string(args)}
		}
		args, _ := json.Marshal(map[string]any{"prompt": task})
		return provider.ToolCall{ID: "t1", Name: "task", Arguments: string(args)}
	}
	provider.Register(kind, func(cfg provider.Config) (provider.Provider, error) {
		return &namedImageProvider{label: map[string]string{"x": "main", "y": "seer"}[cfg.Model], call: call, seen: seen}, nil
	})
	writeFile(t, dir, "reasonix.toml", fmt.Sprintf(`
default_model = "test-model"

[agent]
system_prompt = "BASE"
vision_model = "seer"

[[providers]]
name = "test-model"
kind = %[1]q
model = "x"

[[providers]]
name = "seer"
kind = %[1]q
model = "y"
vision = true
`, kind))
	writeFile(t, dir, ".reasonix/skills/looker.md", "---\ndescription: looks at things\nrunAs: subagent\nread-only: true\n---\nLook at what the task points at.\n")
	approveWorkspace(t, dir)
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	ctrl.SetBypass(true)
	line := "and the second one?"
	if viaTurn {
		line = "look at @.reasonix/attachments/clipboard-pasted.png"
	}
	if err := ctrl.Run(context.Background(), line); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return seen
}

// A delegate is given the image its task text names, wherever the turn that
// spawned it came from: the user's message here named nothing. The parent, which
// cannot read images, is not sent it; the delegate runs on the vision role and
// is told the picture rides its message.
func TestEffectDelegateGetsTheAttachmentItsTaskNames(t *testing.T) {
	for _, tc := range []struct {
		name           string
		skill, viaTurn bool
	}{
		{"task names it", false, false},
		{"read_only_skill names it", true, false},
		{"task, turn carried it", false, true},
		{"read_only_skill, turn carried it", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind := "boot-effect-named-image-" + strings.NewReplacer(" ", "-", "_", "-", ",", "").Replace(tc.name)
			seen := delegateNamingAnAttachment(t, kind, tc.skill, tc.viaTurn)
			child, ok := seen.first("seer")
			if !ok {
				t.Fatal("the delegate never reached the vision model: its images were dropped or it ran on the text-only parent")
			}
			if got := imageCount(child); got != 1 {
				t.Fatalf("the vision delegate was sent %d image(s), want the one", got)
			}
			if !strings.Contains(lastUserText(child), "attached-images") {
				t.Fatalf("the delegate was not told the image rides its message:\n%.400s", lastUserText(child))
			}
			parent, _ := seen.first("main")
			if got := imageCount(parent); got != 0 {
				t.Fatalf("the text-only parent was sent %d image(s)", got)
			}
		})
	}
}
