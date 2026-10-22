package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/sessionstore"
)

type summaryPrefixProvider struct{ requests []provider.Request }

func (*summaryPrefixProvider) Name() string { return "summary-prefix" }
func (p *summaryPrefixProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	p.requests = append(p.requests, req)
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{Type: provider.ChunkText, Text: "## Goal\nA summary"}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func TestSummaryPrefixRequiresAnUnambiguousEndpoint(t *testing.T) {
	msg := func(text string) provider.Message {
		return provider.Message{Role: provider.RoleAssistant, Content: text}
	}
	for _, test := range []struct {
		name           string
		messages, fold []provider.Message
		end            int
	}{
		{"sparse", []provider.Message{msg("a"), msg("kept"), msg("b")}, []provider.Message{msg("a"), msg("b")}, 3},
		{"missing", []provider.Message{msg("a")}, []provider.Message{msg("b")}, 0},
		{"repeated endpoint", []provider.Message{msg("a"), msg("b"), msg("b")}, []provider.Message{msg("a"), msg("b")}, 0},
		{"wrong order", []provider.Message{msg("b"), msg("a")}, []provider.Message{msg("a"), msg("b")}, 0},
		{"empty", []provider.Message{msg("a")}, nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if end := summaryPrefixEnd(test.messages, test.fold); end != test.end {
				t.Fatalf("prefix end = %d, want %d", end, test.end)
			}
		})
	}
}

func TestSummaryPrefixFallsBackForStaleOrOversizedHistoryAndReset(t *testing.T) {
	for _, reason := range []string{"not sent", "rewritten", "reset", "window", "ceiling", "open call"} {
		t.Run(reason, func(t *testing.T) {
			messages := []provider.Message{{Role: provider.RoleSystem, Content: "system"}, {Role: provider.RoleUser, Content: "kept"}, {Role: provider.RoleAssistant, Content: "finding"}}
			window := 100_000
			if reason == "window" {
				window = 20_000
				messages[0].Content = strings.Repeat("prefix ", 4_000)
			}
			if reason == "ceiling" {
				window = 2_000_000
				messages[0].Content = strings.Repeat("prefix ", 400_000)
			}
			if reason == "open call" {
				messages[2].ToolCalls = []provider.ToolCall{{ID: "pending", Name: "read_file", Arguments: "{}"}}
			}
			session := &sessionstore.Session{Messages: messages}
			a := New(&summaryPrefixProvider{}, tool.NewRegistry(), session, Options{ContextWindow: window}, event.Discard)
			req := freezeProviderRequest(provider.Request{Messages: messages, Tools: []provider.ToolSchema{{Name: "read_file"}}})
			if reason != "not sent" {
				a.window().rememberSummaryPrefix(req)
			}
			if reason == "reset" {
				a.sess.win.reset()
			}
			if reason == "rewritten" {
				session.Messages[1].Content = "updated kept constraint"
			}
			fallback := provider.Request{Messages: []provider.Message{{Role: provider.RoleSystem, Content: "independent summary"}}, MaxTokens: summaryOutputMaxTokens, Summary: true}
			got, _ := a.window().withSummaryPrefix(fallback, []provider.Message{messages[2]}, "summary")
			if !reflect.DeepEqual(got, fallback) {
				t.Fatalf("%s did not use the bounded independent request: %+v", reason, got)
			}
		})
	}
}

func TestSummaryReusesTheSentSystemToolsAndMessagePrefix(t *testing.T) {
	p := &summaryPrefixProvider{}
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: "stable instructions"},
		{Role: provider.RoleUser, Content: "task"},
		{Role: provider.RoleAssistant, Content: "first finding"},
		{Role: provider.RoleUser, Content: "retained constraint"},
		{Role: provider.RoleAssistant, Content: "second finding"},
		{Role: provider.RoleUser, Content: "recent question"},
	}
	session := &sessionstore.Session{Messages: messages}
	a := New(p, tool.NewRegistry(), session, Options{ContextWindow: 100_000}, event.Discard)
	req := provider.Request{Messages: messages, Tools: []provider.ToolSchema{{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}}}
	ch, err := a.streamProviderRequest(context.Background(), freezeProviderRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	fold := []provider.Message{messages[2], messages[4]}
	if _, _, err := a.window().summarize(context.Background(), fold, "keep the findings"); err != nil {
		t.Fatal(err)
	}
	got := p.requests[len(p.requests)-1]
	if len(got.Messages) != 6 || !reflect.DeepEqual(got.Messages[:5], messages[:5]) || !reflect.DeepEqual(got.Tools, req.Tools) || !got.Summary {
		t.Fatalf("summary replaced the sent prefix: %+v", got)
	}
	if !reflect.DeepEqual(session.Messages, messages) {
		t.Fatal("summarizing changed canonical history")
	}
}

func TestSummaryPrefixProjectionPreservesCallerTimestamps(t *testing.T) {
	messages := []provider.Message{{Role: provider.RoleAssistant, Content: "finding", CreatedAt: 12345}}
	projected := summaryModelMessages(messages)
	if messages[0].CreatedAt != 12345 || projected[0].CreatedAt != 0 {
		t.Fatalf("projection mutated caller metadata: source=%+v projected=%+v", messages, projected)
	}
}

func TestSummaryToolHistoryReusesOnlyTheSystemAndCatalog(t *testing.T) {
	secret := strings.Repeat("private subagent task ", 100)
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: "stable system"},
		{Role: provider.RoleUser, Content: "task"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "call", Name: "task", Arguments: `{"prompt":"` + secret + `"}`}}},
		{Role: provider.RoleTool, ToolCallID: "call", Name: "task", Content: "finding"},
		{Role: provider.RoleUser, Content: "next"},
	}
	p := &summaryPrefixProvider{}
	a := New(p, tool.NewRegistry(), &sessionstore.Session{Messages: messages}, Options{ContextWindow: 100_000}, event.Discard)
	tools := []provider.ToolSchema{{Name: "task"}}
	a.window().rememberSummaryPrefix(freezeProviderRequest(provider.Request{Messages: messages, Tools: tools}))
	if _, _, err := a.window().summarize(context.Background(), messages[2:4], "focus"); err != nil {
		t.Fatal(err)
	}
	got := p.requests[0]
	if len(got.Messages) != 2 || !reflect.DeepEqual(got.Messages[0], messages[0]) || !reflect.DeepEqual(got.Tools, tools) {
		t.Fatalf("tool history should reuse the stable system and tools: %+v", got)
	}
	for _, m := range got.Messages {
		if strings.Contains(m.Content, secret) || len(m.ToolCalls) > 0 {
			t.Fatal("raw task arguments reached the summarizer")
		}
	}
	if !strings.Contains(got.Messages[1].Content, "[tool task result]\nfinding") || !strings.Contains(got.Messages[1].Content, "prompt") {
		t.Fatalf("sanitized transcript missing: %s", got.Messages[1].Content)
	}
}
