package boot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/model/openai"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/state/sessionstore"
)

type cacheSummaryEffectProvider struct {
	provider.Provider
	mu       sync.Mutex
	requests []provider.Request
	wires    []cacheSummaryWire
}
type cacheSummaryWire struct {
	Messages   []json.RawMessage `json:"messages"`
	Tools      json.RawMessage   `json:"tools"`
	ToolChoice string            `json:"tool_choice"`
	Body       []byte            `json:"-"`
}

func (p *cacheSummaryEffectProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	return p.Provider.Stream(ctx, req)
}

func TestEffectCompactionSharesThePreviouslySentPrefix(t *testing.T) {
	for _, tc := range []struct {
		name            string
		automatic, rich bool
	}{
		{name: "manual sparse retention"}, {name: "automatic pressure", automatic: true}, {name: "tool calls and reasoning", rich: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, dir := statedBootHome(t), testenv.TempDir(t)
			rec := &cacheSummaryEffectProvider{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var wire cacheSummaryWire
				if err = json.Unmarshal(body, &wire); err != nil {
					t.Error(err)
					return
				}
				wire.Body = body
				rec.mu.Lock()
				rec.wires = append(rec.wires, wire)
				n := len(rec.wires)
				summary := rec.requests[len(rec.requests)-1].Summary
				rec.mu.Unlock()
				delta := map[string]any{"content": fmt.Sprintf("Finding %d:\n", n) + strings.Repeat("The tested module returns a typed error and preserves user constraints.\n", 600)}
				finish := "stop"
				if summary {
					delta = map[string]any{"content": "## Goal\nThe module returns typed errors; preserve the public API."}
				} else if tc.rich && n == 1 {
					delta = map[string]any{"reasoning_content": "private-reasoning-marker", "tool_calls": []any{map[string]any{"index": 0, "id": "read-1", "type": "function", "function": map[string]any{"name": "read_file", "arguments": `{"path":"fixture.txt","offset":0,"limit":10}`}}}}
					finish = "tool_calls"
				}
				payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
			}))
			defer srv.Close()
			adapter, err := openai.New(provider.Config{Name: "wire", BaseURL: srv.URL, Model: "test", APIKey: "test", Extra: map[string]any{"protocol": "none"}})
			if err != nil {
				t.Fatal(err)
			}
			rec.Provider = adapter
			kind := "boot-cache-summary-" + t.Name()
			provider.Register(kind, func(provider.Config) (provider.Provider, error) { return rec, nil })
			window, policy := 200000, "context_soft_limit_tokens = -1"
			if tc.automatic {
				window, policy = 96000, "compact_ratio = 0.6"
			}
			writeFile(t, dir, "fixture.txt", "synthetic tool result\n")
			writeFile(t, dir, "reasonix.toml", fmt.Sprintf(`
default_model = "test-model"
[agent]
system_prompt = "BASE"
%s
[[providers]]
name = "test-model"
kind = %q
model = "x"
context_window = %d
max_output_tokens = 1024
`, policy, kind, window))
			approveWorkspaceAt(t, home, dir)
			var pressure atomic.Bool
			sink := event.FuncSink(func(e event.Event) {
				if e.Kind == event.CompactionStarted && e.Compaction.Trigger == agent.CompactionTriggerPressure {
					pressure.Store(true)
				}
			})
			ctrl, err := Build(t.Context(), Options{Home: home, WorkspaceRoot: dir, Sink: sink})
			if err != nil {
				t.Fatal(err)
			}
			defer ctrl.Close()
			ctrl.SetSessionPath(sessionstore.NewSessionPath(ctrl.SessionDir(), ctrl.Label()))
			const retained = "standing constraint: never change the public API"
			prompts := []string{"Explain error handling.", retained, "Explain the edge cases.", "Explain the neighboring behavior.", "Explain remaining limitations."}
			prompts = append(prompts, "Continue checking.", "Finish checking.", "Check one last case.")
			for _, prompt := range prompts {
				if err := ctrl.Run(t.Context(), prompt); err != nil {
					t.Fatal(err)
				}
			}
			if !tc.automatic {
				if _, err := ctrl.Compact(t.Context(), agent.CompactRequest{}); err != nil {
					t.Fatal(err)
				}
			}
			if err := ctrl.Run(t.Context(), "Report the preserved constraints."); err != nil {
				t.Fatal(err)
			}
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if len(rec.requests) != len(rec.wires) {
				t.Fatalf("adapter request count differs: %d / %d", len(rec.requests), len(rec.wires))
			}
			var prior *provider.Request
			priorIndex := -1
			checked := false
			kept := false
			for i := range rec.requests {
				req := &rec.requests[i]
				if !req.Summary {
					prior, priorIndex = req, i
					continue
				}
				if prior == nil {
					t.Fatal("summary has no preceding main turn")
				}
				if len(req.Tools) == 0 {
					continue
				}
				prefix := req.Messages[:len(req.Messages)-1]
				if len(prefix) > len(prior.Messages) || !reflect.DeepEqual(prefix, prior.Messages[:len(prefix)]) || !reflect.DeepEqual(req.Tools, prior.Tools) {
					t.Fatal("Build changed the previously sent prefix")
				}
				wire, prev := rec.wires[i], rec.wires[priorIndex]
				if wire.ToolChoice != "none" || !bytes.Equal(wire.Tools, prev.Tools) {
					t.Fatal("serialized schemas changed or summary tools are enabled")
				}
				head := bytes.Clone(prev.Body[:bytes.Index(prev.Body, prev.Messages[0])])
				for j := range prefix {
					if j > 0 {
						head = append(head, ',')
					}
					head = append(head, prev.Messages[j]...)
				}
				if !bytes.HasPrefix(wire.Body, head) {
					t.Fatal("serialized request root and message prefix changed")
				}
				for j := range prefix {
					if !bytes.Equal(wire.Messages[j], prev.Messages[j]) {
						t.Fatalf("serialized prefix message %d changed", j)
					}
				}
				if tc.rich {
					calls, reasoning := false, false
					for _, m := range prior.Messages {
						calls = calls || len(m.ToolCalls) > 0
						reasoning = reasoning || m.ReasoningContent != ""
					}
					if !calls || !reasoning {
						t.Fatal("fixture did not deliver tool calls and reasoning through Build")
					}
					for _, m := range prefix {
						if m.Role != provider.RoleSystem {
							t.Fatal("rich history reused unsafe conversation bytes")
						}
					}
					tail := req.Messages[len(req.Messages)-1].Content
					if strings.Contains(tail, "private-reasoning-marker") || strings.Contains(tail, `"offset":0`) || !strings.Contains(tail, "synthetic tool result") {
						t.Fatalf("rich transcript lost tool output or leaked reasoning/arguments: %.2000s", tail)
					}
				} else {
					for _, m := range prefix {
						if m.Role == provider.RoleUser && strings.Contains(m.Content, retained) {
							kept = true
						}
					}
				}
				checked = true
			}
			if tc.automatic && !pressure.Load() {
				t.Fatal("fixture never reached pressure compaction")
			}
			if !checked {
				t.Fatal("no summary reused a sent prefix through the real Build stack")
			}
			if !tc.rich && !tc.automatic && !kept {
				t.Fatal("sparse summary prefix did not include the retained user turn")
			}
			if !tc.rich {
				last := rec.requests[len(rec.requests)-1]
				found := false
				for _, m := range last.Messages {
					if m.Role == provider.RoleUser && strings.Contains(m.Content, retained) {
						found = true
					}
				}
				if !found {
					t.Fatal("sparse fold removed the retained user turn")
				}
			}
		})
	}
}
