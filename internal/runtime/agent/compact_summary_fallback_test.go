package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/model/providerbroker"
	"reasonix/internal/state/sessionstore"
)

type fallbackReply struct {
	text, finish string
	err          error
}
type summaryFallbackProvider struct {
	replies  []fallbackReply
	requests []provider.Request
}

func (*summaryFallbackProvider) Name() string { return "summary-fallback" }
func (p *summaryFallbackProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	if !req.Summary {
		ch := make(chan provider.Chunk)
		close(ch)
		return ch, nil
	}
	i := len(p.requests)
	p.requests = append(p.requests, req)
	if i >= len(p.replies) {
		return nil, errors.New("unexpected extra summary")
	}
	r := p.replies[i]
	if r.err != nil {
		return nil, r.err
	}
	ch := make(chan provider.Chunk, 3)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: r.text}
	ch <- provider.Chunk{Type: provider.ChunkUsage, Usage: &provider.Usage{PromptTokens: 100, CompletionTokens: 10, TotalTokens: 110, CacheHitTokens: 40, CacheMissTokens: 60, CacheWriteTokens: 20, CacheWriteBilledTokens: 25, RequestCount: 1, FinishReason: r.finish}}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func TestSummaryPrefixFallsBackOnceAndAccountsForEveryAttempt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replies []fallbackReply
		prefix  bool
		want    int
		wantErr bool
	}{
		{"off task then valid", []fallbackReply{{text: "I will edit the code"}, {text: "## Goal\nvalid"}}, true, 2, false},
		{"both off task", []fallbackReply{{text: "I will edit"}, {text: "still editing"}}, true, 2, true},
		{"fallback error", []fallbackReply{{text: "I will edit"}, {err: errors.New("offline")}}, true, 2, true},
		{"legacy off task", []fallbackReply{{text: "I will edit"}}, false, 1, true},
		{"truncated then off task", []fallbackReply{{text: "## Goal\ncut", finish: "length"}, {text: "I will edit"}, {text: "## Goal\nvalid"}}, true, 3, false},
		{"off task then truncated", []fallbackReply{{text: "I will edit"}, {text: "## Goal\ncut", finish: "length"}, {text: "## Goal\nvalid"}}, true, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := []provider.Message{{Role: provider.RoleSystem, Content: "stable"}, {Role: provider.RoleUser, Content: "task"}, {Role: provider.RoleAssistant, Content: "finding"}, {Role: provider.RoleUser, Content: "next"}}
			p := &summaryFallbackProvider{replies: tc.replies}
			var frames []event.Event
			a := New(p, tool.NewRegistry(), &sessionstore.Session{Messages: messages}, Options{ContextWindow: 100_000, MaxOutputTokens: 4 * summaryOutputMaxTokens}, event.FuncSink(func(e event.Event) { frames = append(frames, e) }))
			if tc.prefix {
				ch, err := a.streamProviderRequest(t.Context(), freezeProviderRequest(provider.Request{Messages: messages, Tools: []provider.ToolSchema{{Name: "read_file"}}}))
				if err != nil {
					t.Fatal(err)
				}
				for range ch {
				}
			}
			_, _, usage, _, err := a.window().runCompactionSummary(t.Context(), messages[2:3], "")
			if (err != nil) != tc.wantErr || len(p.requests) != tc.want {
				t.Fatalf("calls=%d err=%v; want calls=%d error=%v", len(p.requests), err, tc.want, tc.wantErr)
			}
			if tc.prefix && len(p.requests[0].Tools) == 0 {
				t.Fatal("fixture did not exercise reuse")
			}
			switched := false
			for _, req := range p.requests {
				if len(req.Tools) == 0 {
					switched = true
					if len(req.Messages) != 2 || req.Messages[0].Content != summarySystemPrompt {
						t.Fatal("fallback was not the independent shape")
					}
				} else if switched {
					t.Fatal("retry regained prefix after fallback")
				}
			}
			count := 0
			tokens := 0
			restarts := 0
			for _, e := range frames {
				if e.Kind == event.Usage && e.Usage != nil {
					count++
					tokens += e.Usage.TotalTokens
				}
				if e.Kind == event.CompactionProgress && e.Text == "" {
					restarts++
				}
			}
			if usage == nil || usage.TotalTokens != tokens || usage.RequestCount != count || usage.CacheWriteTokens != 20*count || usage.CacheWriteBilledTokens != 25*float64(count) {
				t.Fatalf("accounting usage=%+v events=%d tokens=%d", usage, count, tokens)
			}
			if restarts != tc.want-1 {
				t.Fatalf("progress restarts=%d want=%d", restarts, tc.want-1)
			}
			if !reflect.DeepEqual(a.sess.conversation.Messages, messages) {
				t.Fatal("summary changed canonical history")
			}
		})
	}
}

func TestSummaryPrefixFallsBackThroughLegacyBroker(t *testing.T) {
	var posts []provider.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == providerbroker.PathCatalog {
			_, _ = w.Write([]byte(`{"providers":[]}`))
			return
		}
		var body struct {
			Request provider.Request `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		posts = append(posts, body.Request)
		if len(posts) > 1 && len(body.Request.Tools) > 0 {
			t.Error("schema-bearing summary reached legacy broker")
		}
		_, _ = w.Write([]byte(`{"type":0,"text":"## Goal\nPreserve the public API"}` + "\n" + `{"type":5,"usage":{"PromptTokens":100,"CompletionTokens":10,"TotalTokens":110,"RequestCount":1}}` + "\n" + `{"type":6}` + "\n"))
	}))
	defer srv.Close()
	p, err := providerbroker.NewClient(srv.URL, "test-token", srv.Client()).Resolve(provider.Selection{Ref: "p/m"})
	if err != nil {
		t.Fatal(err)
	}
	messages := []provider.Message{{Role: provider.RoleSystem, Content: "stable"}, {Role: provider.RoleUser, Content: "task"}, {Role: provider.RoleAssistant, Content: "finding"}, {Role: provider.RoleUser, Content: "next"}}
	var events []event.Event
	a := New(p, tool.NewRegistry(), &sessionstore.Session{Messages: messages}, Options{ContextWindow: 100000}, event.FuncSink(func(e event.Event) { events = append(events, e) }))
	ch, err := a.streamProviderRequest(t.Context(), freezeProviderRequest(provider.Request{Messages: messages, Tools: []provider.ToolSchema{{Name: "read_file"}}}))
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	_, _, usage, _, err := a.window().runCompactionSummary(t.Context(), messages[2:3], "")
	if err != nil || len(posts) != 2 {
		t.Fatalf("posts=%d err=%v", len(posts), err)
	}
	if len(posts[1].Messages) != 2 || posts[1].Messages[0].Content != summarySystemPrompt || len(posts[1].Tools) != 0 {
		t.Fatal("legacy broker did not receive the independent shape")
	}
	billed := 0
	for _, e := range events {
		if e.Kind == event.Usage {
			billed++
		}
	}
	if usage == nil || usage.RequestCount != 1 || usage.TotalTokens != 110 || billed != 1 {
		t.Fatalf("capability refusal was billed as a model request: usage=%+v events=%d", usage, billed)
	}
}
