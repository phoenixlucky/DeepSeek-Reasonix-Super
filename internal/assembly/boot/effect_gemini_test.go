package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

// geminiServer answers the Interactions API with one thought and one answer, and
// records what each request carried. The wire is built from Google's documentation
// (https://ai.google.dev/gemini-api/docs/interactions/streaming, read 2026-10-17)
// and was not run against the real endpoint.
type geminiServer struct {
	mu      sync.Mutex
	bodies  []string
	headers []http.Header
	paths   []string
}

func (g *geminiServer) handler(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	g.mu.Lock()
	g.bodies = append(g.bodies, string(b))
	g.headers = append(g.headers, r.Header.Clone())
	g.paths = append(g.paths, r.URL.Path)
	n := len(g.bodies)
	g.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	events := []string{
		`{"event_type":"interaction.created","interaction":{"id":"i","status":"in_progress"}}`,
		`{"event_type":"step.start","index":0,"step":{"type":"thought"}}`,
		fmt.Sprintf(`{"event_type":"step.delta","index":0,"delta":{"type":"thought_summary","content":{"type":"text","text":"thinking %d"}}}`, n),
		fmt.Sprintf(`{"event_type":"step.delta","index":0,"delta":{"type":"thought_signature","signature":"SIG-%d"}}`, n),
		`{"event_type":"step.stop","index":0}`,
		`{"event_type":"step.start","index":1,"step":{"type":"model_output"}}`,
		fmt.Sprintf(`{"event_type":"step.delta","index":1,"delta":{"type":"text","text":"answer %d"}}`, n),
		`{"event_type":"step.stop","index":1}`,
		`{"event_type":"interaction.completed","interaction":{"id":"i","status":"completed","usage":{"total_tokens":30,"total_input_tokens":10,"total_output_tokens":5,"total_thought_tokens":15,"total_cached_tokens":4}}}`,
	}
	for _, e := range events {
		var probe struct {
			EventType string `json:"event_type"`
		}
		_ = json.Unmarshal([]byte(e), &probe)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", probe.EventType, e)
	}
	fmt.Fprint(w, "event: done\ndata: [DONE]\n\n")
}

// A thought the model produced rides the next request exactly as it came, in
// order, beside the answer it belonged to; nothing in the request is a sampling
// parameter, and the request is stateless.
func TestEffectAGeminiThoughtIsReplayedOnTheNextRequest(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	g := &geminiServer{}
	srv := httptest.NewServer(http.HandlerFunc(g.handler))
	defer srv.Close()
	writeFile(t, filepath.Dir(config.UserConfigPath()), ".env", "GEMINI_EFFECT_KEY=k-effect\n")
	writeFile(t, filepath.Dir(config.UserConfigPath()), "config.toml", fmt.Sprintf(`default_model = "gem/gemini-3.8-flash"
[agent]
system_prompt = "BASE"
[[providers]]
name = "gem"
kind = "gemini"
base_url = %q
model = "gemini-3.8-flash"
models = ["gemini-3.8-flash"]
api_key_env = "GEMINI_EFFECT_KEY"
`, srv.URL+"/v1beta"))
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard, Model: "gem/gemini-3.8-flash", WorkspaceRoot: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	for _, prompt := range []string{"first question", "second question"} {
		if err := ctrl.Run(context.Background(), prompt); err != nil {
			t.Fatalf("Run(%q): %v", prompt, err)
		}
	}
	if len(g.bodies) != 2 {
		t.Fatalf("%d requests, want 2", len(g.bodies))
	}
	for i, p := range g.paths {
		if p != "/v1beta/interactions" || g.headers[i].Get("x-goog-api-key") != "k-effect" {
			t.Fatalf("request %d: path=%q key=%q headers=%v", i, p, g.headers[i].Get("x-goog-api-key"), g.headers[i])
		}
	}
	first, second := g.bodies[0], g.bodies[1]
	for _, banned := range []string{"temperature", "top_p"} {
		if strings.Contains(first, banned) {
			t.Fatalf("a sampling parameter reached the wire: %s", first)
		}
	}
	for _, want := range []string{`"store":false`, `"stream":true`, `"system_instruction":"BASE`, `"type":"user_input"`, `"type":"function"`} {
		if !strings.Contains(first, want) {
			t.Fatalf("the first request lacks %s:\n%.600s", want, first)
		}
	}
	replay := `{"type":"thought","summary":[{"type":"text","text":"thinking 1"}],"signature":"SIG-1"},` +
		`{"type":"model_output","content":[{"type":"text","text":"answer 1"}]},` +
		`{"type":"user_input","content":[`
	if !strings.Contains(second, replay) || !strings.Contains(second, "second question") {
		t.Fatalf("the first turn did not come back verbatim and in order:\n%s", second)
	}
}
