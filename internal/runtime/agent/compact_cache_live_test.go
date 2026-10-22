//go:build live_compaction

package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/model/openai"
	"reasonix/internal/state/sessionstore"
)

type liveSummaryRecorder struct {
	provider.Provider
	requests []provider.Request
	results  []liveSummaryResult
}
type liveSummaryResult struct {
	Kind         string          `json:"kind"`
	Milliseconds int64           `json:"milliseconds"`
	Digest       bool            `json:"digest"`
	Grounded     bool            `json:"grounded"`
	ToolCalls    int             `json:"tool_calls"`
	Usage        *provider.Usage `json:"usage"`
}

func (p *liveSummaryRecorder) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.requests = append(p.requests, req)
	start := time.Now()
	ch, err := p.Provider.Stream(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan provider.Chunk)
	go func() {
		defer close(out)
		var text strings.Builder
		var usage *provider.Usage
		calls := 0
		for c := range ch {
			if c.Type == provider.ChunkToolCall {
				calls++
			}
			if c.Type == provider.ChunkText {
				text.WriteString(c.Text)
			}
			if c.Type == provider.ChunkUsage {
				usage = c.Usage
			}
			select {
			case out <- c:
			case <-ctx.Done():
				return
			}
		}
		kind := "warm"
		if req.Summary {
			kind = "reuse"
			if len(req.Tools) == 0 {
				kind = "independent"
			}
		}
		p.results = append(p.results, liveSummaryResult{Kind: kind, Milliseconds: time.Since(start).Milliseconds(), Digest: hasDigestHeading(text.String()), Grounded: strings.Contains(text.String(), "ParseDuration") && strings.Contains(strings.ToLower(text.String()), "zero"), ToolCalls: calls, Usage: usage})
	}()
	return out, nil
}

func TestLiveCompactionPrefix(t *testing.T) {
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		t.Skip("DEEPSEEK_API_KEY is unset")
	}
	model := os.Getenv("REASONIX_LIVE_MODEL")
	if model == "" {
		model = "deepseek-flash"
	}
	savedTransport := http.DefaultTransport
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	http.DefaultTransport = transport
	t.Cleanup(func() { transport.CloseIdleConnections(); http.DefaultTransport = savedTransport })
	p, err := openai.New(provider.Config{Name: "deepseek", BaseURL: "https://api.deepseek.com", Model: model, APIKey: key, Extra: map[string]any{"thinking": "disabled"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, rich := range []bool{false, true} {
		for trial := range 3 {
			rec := &liveSummaryRecorder{Provider: p}
			messages := []provider.Message{
				{Role: provider.RoleSystem, Content: "You are a coding agent. Preserve the public API. " + strings.Repeat("Keep changes narrow and run relevant tests. ", 100)},
				{Role: provider.RoleUser, Content: "Investigate why ParseDuration accepts negative values. Preserve function names and return a typed error."},
				{Role: provider.RoleAssistant, Content: "Inspected duration.go: ParseDuration lacks a negative-value guard. Plan: reject values below zero and add boundary tests."},
				{Role: provider.RoleUser, Content: "Continue; ensure zero remains valid."},
			}
			if rich {
				messages = append(messages[:2], provider.Message{Role: provider.RoleAssistant, ReasoningContent: "Inspect the input guard.", ToolCalls: []provider.ToolCall{{ID: "call_read", Name: "read_file", Arguments: `{"path":"duration.go","private_payload":"synthetic-argument-marker"}`}}}, provider.Message{Role: provider.RoleTool, Name: "read_file", ToolCallID: "call_read", Content: "func ParseDuration(v int) (int,error) { return v,nil }"}, messages[2], messages[3])
			}
			schemas := []provider.ToolSchema{{Name: "read_file", Description: "Read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}}
			a := New(rec, tool.NewRegistry(), &sessionstore.Session{Messages: messages}, Options{ContextWindow: 128000, MaxOutputTokens: 1024}, event.Discard)
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			ch, err := a.streamProviderRequest(ctx, freezeProviderRequest(provider.Request{Messages: messages, Tools: schemas, MaxTokens: 64}))
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			for c := range ch {
				if c.Type == provider.ChunkError {
					cancel()
					t.Fatal(c.Err)
				}
			}
			fold := messages[2 : len(messages)-1]
			_, _, _, _, err = a.window().runCompactionSummary(ctx, fold, "")
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			_, _, err = a.window().summarizeAtMode(ctx, fold, "", 1024, 0, false)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rec.results {
				if r.Kind != "warm" && (!r.Digest || !r.Grounded || r.ToolCalls != 0 || r.Usage == nil || r.Usage.FinishReason != "stop") {
					t.Fatalf("summary contract failed: %+v", r)
				}
			}
			data, _ := json.Marshal(struct {
				Model   string              `json:"model"`
				Rich    bool                `json:"rich"`
				Trial   int                 `json:"trial"`
				Results []liveSummaryResult `json:"results"`
			}{model, rich, trial + 1, rec.results})
			t.Log(string(data))
		}
	}
}
