package boot

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

// videoProvider looks for a way to see a video, calls what it finds, then answers.
type videoProvider struct {
	mu   sync.Mutex
	reqs []provider.Request
}

// The provider registry takes a kind once; each run points it at its own recorder.
var (
	registerVideoProvider sync.Once
	videoCurrent          atomic.Pointer[videoProvider]
)

func (p *videoProvider) Name() string { return "boot-video" }

func (p *videoProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	round := len(p.reqs)
	p.mu.Unlock()
	ch := make(chan provider.Chunk, 2)
	switch round {
	case 1:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "s1", Name: "use_capability", Arguments: `{"action":"search","query":"sample frames from a video file"}`}}
	case 2:
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "v1", Name: "use_capability", Arguments: `{"action":"call","capability_id":"tool:read_video","arguments":{"path":"clip.mp4","frames":3}}`}}
	default:
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (p *videoProvider) requests() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.reqs...)
}

// videoRun follows a video from the workspace to the requests a vision model is
// sent, under the given user config. The stand-ins for ffmpeg and ffprobe live
// inside the workspace, which an OS sandbox always shows to the programs it
// wraps; a directory elsewhere under /tmp is invisible to them. ran says whether
// either stand-in was started at all.
func videoRun(t *testing.T, userConfig string) (reqs []provider.Request, ran bool) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in programs are shell scripts")
	}
	home := statedBootHome(t)
	dir := testenv.TempDir(t)
	if userConfig != "" {
		writeUserConfigAt(t, home, userConfig)
	}

	var frame bytes.Buffer
	if err := jpeg.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 32, 18)), nil); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, ".stubs")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	framePath := filepath.Join(bin, "frame.jpg")
	if err := os.WriteFile(framePath, frame.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(bin, "ran")
	script := func(name, body string) {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script("ffprobe", `: > '`+marker+`'; echo '{"streams":[{"codec_type":"video","width":640,"height":360}],"format":{"duration":"30.0"}}'`)
	script("ffmpeg", `: > '`+marker+`'; cat '`+framePath+`'`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	mp4 := append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, make([]byte, 64)...)
	if err := os.WriteFile(filepath.Join(dir, "clip.mp4"), mp4, 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &videoProvider{}
	videoCurrent.Store(rec)
	registerVideoProvider.Do(func() {
		provider.Register("boot-video", func(provider.Config) (provider.Provider, error) { return videoCurrent.Load(), nil })
	})
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[codegraph]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-video"
model = "x"
vision = true
`)
	approveWorkspaceAt(t, home, dir)

	// "ask" is how an unattended run fails closed: a call that needs approval is refused, not waved through.
	ctrl, err := Build(context.Background(), Options{Home: home, WorkspaceRoot: dir, Sink: event.Discard, HeadlessApprovalMode: "ask"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "what is in clip.mp4?"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	_, statErr := os.Stat(marker)
	return agentRequests(rec.requests()), statErr == nil
}

func toolMessages(req provider.Request) (images []string, told []string) {
	for _, m := range req.Messages {
		if m.Role != provider.RoleTool {
			continue
		}
		told = append(told, m.Content)
		images = append(images, m.Images...)
	}
	return images, told
}

// TestEffectReadVideoFramesReachTheProviderAsImages: the tool is found through
// the capability search and not added to the schema prefix, it runs against the
// real assembly's read boundary, and its frames arrive on the tool message as
// images. The user config allows it, so what the machine's sandbox is does not
// decide the outcome.
func TestEffectReadVideoFramesReachTheProviderAsImages(t *testing.T) {
	reqs, _ := videoRun(t, "[permissions]\nallow = [\"read_video\"]\n")
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3", len(reqs))
	}
	for _, spec := range reqs[0].Tools {
		if spec.Name == "read_video" {
			t.Fatal("read_video joined the schema prefix every session pays for; it is reached through use_capability")
		}
	}
	_, searched := toolMessages(reqs[1])
	if !strings.Contains(strings.Join(searched, "\n"), "tool:read_video") {
		t.Fatal("the capability search did not offer tool:read_video for a video")
	}
	images, told := toolMessages(reqs[2])
	if len(images) != 3 {
		t.Fatalf("the last request carried %d frames, want 3; tool messages: %q", len(images), told)
	}
	if w := imageWidth(t, images[0]); w != 32 {
		t.Errorf("frame width %d, want the 32 the stand-in produced", w)
	}
	joined := strings.Join(told, "\n")
	if !strings.Contains(joined, "[frame 1/3 at ") || !strings.Contains(joined, "640x360") {
		t.Errorf("the tool text does not label the frames:\n%s", joined)
	}
}

// With the OS sandbox off and no rule of the user's own, the first use is not
// waved through: an unattended run asks and fails closed, and nothing is started.
func TestEffectReadVideoAsksFirstWithoutASandbox(t *testing.T) {
	reqs, ran := videoRun(t, "[sandbox]\nbash = \"off\"\n")
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3", len(reqs))
	}
	if ran {
		t.Error("ffprobe or ffmpeg was started although the call needed approval")
	}
	images, told := toolMessages(reqs[2])
	if len(images) != 0 {
		t.Errorf("frames reached the model without approval: %d", len(images))
	}
	if len(told) == 0 {
		t.Fatal("the model was told nothing about the refused call")
	}
}

// The user's own rule, in the user config, settles it even with no sandbox.
func TestEffectReadVideoUserAllowOverridesTheDefaultAsk(t *testing.T) {
	reqs, ran := videoRun(t, "[sandbox]\nbash = \"off\"\n[permissions]\nallow = [\"read_video\"]\n")
	if len(reqs) != 3 {
		t.Fatalf("requests = %d, want 3", len(reqs))
	}
	images, told := toolMessages(reqs[2])
	if !ran || len(images) != 3 {
		t.Fatalf("ran %v, %d frames; tool messages: %q", ran, len(images), told)
	}
}
