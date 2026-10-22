package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"reasonix/internal/contract/provider"
)

// The two files are streams captured from DeepSeek's official OpenAI-compatible
// endpoint (deepseek-flash, a ten-token prompt): usage is null on every chunk
// except the last one, the one that carries finish_reason.
func captured(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func deepSeekClient(srv *httptest.Server) *client {
	return &client{
		name: "deepseek", apiKey: staticKey("k"), baseURL: srv.URL, chatURL: srv.URL + "/chat/completions",
		prefixChatURL: srv.URL + "/beta/chat/completions", model: "deepseek-v4-flash", deepseek: true,
		effort: "high", http: srv.Client(), idleTimeout: defaultStreamIdleTimeout,
	}
}

func drain(t *testing.T, c *client) (text string, usages []*provider.Usage) {
	t.Helper()
	ch, err := c.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkText:
			b.WriteString(chunk.Text)
		case provider.ChunkUsage:
			usages = append(usages, chunk.Usage)
		case provider.ChunkError:
			t.Fatalf("stream error: %v", chunk.Err)
		}
	}
	return b.String(), usages
}

func TestARealDeepSeekStreamIsCountedOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, captured(t, "deepseek_stream_stop.sse"))
	}))
	defer srv.Close()
	text, usages := drain(t, deepSeekClient(srv))
	if text != "Hello there, how are you?" {
		t.Fatalf("text = %q", text)
	}
	if len(usages) != 1 {
		t.Fatalf("usage chunks = %d, want 1", len(usages))
	}
	u := usages[0]
	if u.PromptTokens != 10 || u.CompletionTokens != 7 || u.TotalTokens != 17 || u.CacheHitTokens != 0 || u.CacheMissTokens != 10 || u.RequestCount != 1 {
		t.Fatalf("usage = %+v, want 10 prompt, 7 completion, 0 hit, 10 miss, one request", u)
	}
}

// A reply cut at the output limit is continued by a second request; each request
// is billed on its own, so the answer carries both and counts two requests.
func TestARealDeepSeekLengthCutAndItsContinuationAreBothCounted(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, captured(t, "deepseek_stream_length.sse"))
			return
		}
		_, _ = io.WriteString(w, captured(t, "deepseek_stream_stop.sse"))
	}))
	defer srv.Close()
	_, usages := drain(t, deepSeekClient(srv))
	if calls.Load() != 2 {
		t.Fatalf("requests = %d, want the cut one and its continuation", calls.Load())
	}
	if len(usages) != 1 {
		t.Fatalf("usage chunks = %d, want one merged answer", len(usages))
	}
	u := usages[0]
	if u.PromptTokens != 20 || u.CompletionTokens != 10 || u.CacheMissTokens != 20 || u.CacheHitTokens != 0 || u.RequestCount != 2 {
		t.Fatalf("usage = %+v, want 20 prompt, 10 completion, 20 miss, two requests", u)
	}
}
