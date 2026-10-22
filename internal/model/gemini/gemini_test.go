package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/provider"
)

// sse writes events the way the streaming page shows them: an event line, a data
// line whose JSON repeats the name in event_type, a blank line.
func sse(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		var probe struct {
			EventType string `json:"event_type"`
		}
		_ = json.Unmarshal([]byte(e), &probe)
		b.WriteString("event: " + probe.EventType + "\ndata: " + e + "\n\n")
	}
	b.WriteString("event: done\ndata: [DONE]\n\n")
	return b.String()
}

func newTestClient(t *testing.T, srv *httptest.Server, extra map[string]any) provider.Provider {
	t.Helper()
	if extra == nil {
		extra = map[string]any{}
	}
	p, err := New(provider.Config{Name: "g", BaseURL: srv.URL + "/v1beta", Model: "gemini-3.8-flash", APIKey: "k-test", Extra: extra})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func collect(t *testing.T, p provider.Provider, req provider.Request) []provider.Chunk {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, err := p.Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var out []provider.Chunk
	for c := range ch {
		out = append(out, c)
	}
	return out
}

func serve(t *testing.T, body string, seen *[]byte, header *http.Header, path *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen, _ = io.ReadAll(r.Body)
		}
		if header != nil {
			*header = r.Header.Clone()
		}
		if path != nil {
			*path = r.URL.Path
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const textStream = `{"event_type":"interaction.created","interaction":{"id":"i1","status":"in_progress","object":"interaction","model":"gemini-3.8-flash"}}`

// https://ai.google.dev/gemini-api/docs/interactions/streaming (read 2026-10-17)
func TestTextStreamsLiveAndUsageComesFromTheCompletedInteraction(t *testing.T) {
	body := sse(textStream,
		`{"event_type":"step.start","index":0,"step":{"type":"model_output"}}`,
		`{"event_type":"step.delta","index":0,"delta":{"type":"text","text":"Hel"}}`,
		`{"event_type":"step.delta","index":0,"delta":{"type":"text","text":"lo"}}`,
		`{"event_type":"step.stop","index":0}`,
		`{"event_type":"interaction.completed","interaction":{"id":"i1","status":"completed","usage":{"total_tokens":530,"total_input_tokens":62,"total_output_tokens":171,"total_thought_tokens":297,"total_cached_tokens":20}}}`,
	)
	var seen []byte
	var header http.Header
	var path string
	srv := serve(t, body, &seen, &header, &path)
	got := collect(t, newTestClient(t, srv, nil), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}})

	var text strings.Builder
	var usage *provider.Usage
	var items []string
	done := false
	for _, c := range got {
		switch c.Type {
		case provider.ChunkText:
			text.WriteString(c.Text)
		case provider.ChunkUsage:
			usage = c.Usage
		case provider.ChunkResponsesItem:
			items = append(items, string(c.ResponsesItem))
		case provider.ChunkDone:
			done = true
		case provider.ChunkError:
			t.Fatalf("error: %v", c.Err)
		}
	}
	if text.String() != "Hello" || !done {
		t.Fatalf("text=%q done=%v", text.String(), done)
	}
	if usage == nil || usage.PromptTokens != 62 || usage.CompletionTokens != 171+297 || usage.ReasoningTokens != 297 ||
		usage.CacheHitTokens != 20 || usage.CacheMissTokens != 42 || usage.TotalTokens != 530 || usage.FinishReason != "stop" {
		t.Fatalf("usage = %+v", usage)
	}
	if len(items) != 1 || items[0] != `{"type":"model_output","content":[{"type":"text","text":"Hello"}]}` {
		t.Fatalf("items = %v", items)
	}
	if path != "/v1beta/interactions" || header.Get("x-goog-api-key") != "k-test" || header.Get("Accept") != "text/event-stream" {
		t.Fatalf("path=%q key=%q accept=%q", path, header.Get("x-goog-api-key"), header.Get("Accept"))
	}
}

func TestAThoughtAndAStreamedFunctionCallAreEmittedAndKeptForReplay(t *testing.T) {
	body := sse(textStream,
		`{"event_type":"step.start","index":0,"step":{"type":"thought"}}`,
		`{"event_type":"step.delta","index":0,"delta":{"type":"thought_summary","content":{"type":"text","text":"Need the weather"}}}`,
		`{"event_type":"step.delta","index":0,"delta":{"type":"thought_signature","signature":"SIG-1"}}`,
		`{"event_type":"step.stop","index":0}`,
		`{"event_type":"step.start","index":1,"step":{"type":"function_call","id":"fc_1","name":"get_weather","arguments":{}}}`,
		`{"event_type":"step.delta","index":1,"delta":{"type":"arguments_delta","arguments":"{\"location\": \"Par"}}`,
		`{"event_type":"step.delta","index":1,"delta":{"type":"arguments_delta","arguments":"is\"}"}}`,
		`{"event_type":"step.stop","index":1}`,
		`{"event_type":"interaction.completed","interaction":{"id":"i1","status":"requires_action","usage":{"total_tokens":10,"total_input_tokens":4,"total_output_tokens":2,"total_thought_tokens":4}}}`,
	)
	got := collect(t, newTestClient(t, serve(t, body, nil, nil, nil), nil), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "weather?"}}})

	var reasoning, sig string
	var start, call *provider.ToolCall
	var items []string
	var usage *provider.Usage
	for _, c := range got {
		switch c.Type {
		case provider.ChunkReasoning:
			reasoning += c.Text
			sig += c.Signature
		case provider.ChunkToolCallStart:
			start = c.ToolCall
		case provider.ChunkToolCall:
			call = c.ToolCall
		case provider.ChunkResponsesItem:
			items = append(items, string(c.ResponsesItem))
		case provider.ChunkUsage:
			usage = c.Usage
		case provider.ChunkError:
			t.Fatalf("error: %v", c.Err)
		}
	}
	if reasoning != "Need the weather" || sig != "SIG-1" {
		t.Fatalf("reasoning=%q sig=%q", reasoning, sig)
	}
	if start == nil || start.ID != "fc_1" || start.Name != "get_weather" || call == nil || call.Arguments != `{"location": "Paris"}` {
		t.Fatalf("start=%+v call=%+v", start, call)
	}
	want := []string{
		`{"type":"thought","summary":[{"type":"text","text":"Need the weather"}],"signature":"SIG-1"}`,
		`{"type":"function_call","id":"fc_1","name":"get_weather","arguments":{"location":"Paris"}}`,
	}
	if len(items) != 2 || items[0] != want[0] || items[1] != want[1] {
		t.Fatalf("items = %v", items)
	}
	if usage == nil || usage.FinishReason != "tool_calls" {
		t.Fatalf("usage = %+v", usage)
	}
}

func rawItems(items ...string) []json.RawMessage {
	var out []json.RawMessage
	for _, i := range items {
		out = append(out, json.RawMessage(i))
	}
	return out
}

// A stateless follow-up sends the thought back exactly as it was received, in
// order, with the call and the result after it
// (https://ai.google.dev/gemini-api/docs/thinking, read 2026-10-17).
func TestAStoredTurnIsReplayedVerbatimAndInOrder(t *testing.T) {
	var seen []byte
	srv := serve(t, sse(`{"event_type":"interaction.completed","interaction":{"status":"completed"}}`), &seen, nil, nil)
	assistant := provider.Message{
		Role:      provider.RoleAssistant,
		ToolCalls: []provider.ToolCall{{ID: "fc_1", Name: "get_weather", Arguments: `{"location": "Paris"}`}},
		ResponsesItems: rawItems(
			`{"type":"thought","summary":[{"type":"text","text":"Need the weather"}],"signature":"SIG-1"}`,
			`{"type":"function_call","id":"fc_1","name":"get_weather","arguments":{"location": "Paris"},"signature":"CALL-SIG"}`,
		),
	}
	collect(t, newTestClient(t, srv, nil), provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "be brief"},
			{Role: provider.RoleUser, Content: "weather?"},
			assistant,
			{Role: provider.RoleTool, ToolCallID: "fc_1", Name: "get_weather", Content: `{"weather":"sunny"}`},
		},
		Tools: []provider.ToolSchema{{Name: "get_weather", Description: "Gets the weather.", Parameters: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`)}},
	})
	want := `{"model":"gemini-3.8-flash","input":[` +
		`{"type":"user_input","content":[{"type":"text","text":"weather?"}]},` +
		`{"type":"thought","summary":[{"type":"text","text":"Need the weather"}],"signature":"SIG-1"},` +
		`{"type":"function_call","id":"fc_1","name":"get_weather","arguments":{"location":"Paris"},"signature":"CALL-SIG"},` +
		`{"type":"function_result","call_id":"fc_1","name":"get_weather","result":[{"type":"text","text":"{\"weather\":\"sunny\"}"}]}],` +
		`"system_instruction":"be brief",` +
		`"tools":[{"type":"function","name":"get_weather","description":"Gets the weather.","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}],` +
		`"stream":true,"store":false,"generation_config":{"max_output_tokens":16384}}`
	if string(seen) != want {
		t.Fatalf("request body\n got: %s\nwant: %s", seen, want)
	}
}

func TestAnEditedTurnKeepsItsThoughtsAndRebuildsTheRest(t *testing.T) {
	c := &client{}
	got, _ := json.Marshal(assistantSteps(provider.Message{
		Role:      provider.RoleAssistant,
		Content:   "folded summary",
		ToolCalls: []provider.ToolCall{{ID: "fc_9", Name: "x", Arguments: `not json`}},
		ResponsesItems: rawItems(
			`{"type":"thought","signature":"SIG-1"}`,
			`{"type":"function_call","id":"fc_1","name":"x","arguments":{}}`,
		),
	}))
	_ = c
	want := `[{"type":"thought","signature":"SIG-1"},` +
		`{"type":"model_output","content":[{"type":"text","text":"folded summary"}]},` +
		`{"type":"function_call","id":"fc_9","name":"x","arguments":{}}]`
	if string(got) != want {
		t.Fatalf("steps\n got: %s\nwant: %s", got, want)
	}
}

func TestATurnFromAnotherProviderBecomesPlainSteps(t *testing.T) {
	got, _ := json.Marshal(assistantSteps(provider.Message{Role: provider.RoleAssistant, Content: "answer"}))
	if string(got) != `[{"type":"model_output","content":[{"type":"text","text":"answer"}]}]` {
		t.Fatalf("steps = %s", got)
	}
}

func TestTemperatureIsNeverSentAndTheThinkingLevelFollowsTheEffort(t *testing.T) {
	var seen []byte
	srv := serve(t, sse(`{"event_type":"interaction.completed","interaction":{"status":"completed"}}`), &seen, nil, nil)
	temp := 0.2
	collect(t, newTestClient(t, srv, map[string]any{"effort": "medium"}), provider.Request{
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}, Temperature: &temp, EffortOverride: "high", MaxTokens: 4096,
	})
	var body map[string]any
	if err := json.Unmarshal(seen, &body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(seen), "temperature") || strings.Contains(string(seen), "top_p") {
		t.Fatalf("a sampling parameter was sent: %s", seen)
	}
	cfg := body["generation_config"].(map[string]any)
	if cfg["thinking_level"] != "high" || cfg["thinking_summaries"] != "auto" || cfg["max_output_tokens"] != float64(4096) {
		t.Fatalf("generation_config = %v", cfg)
	}
	if body["store"] != false || body["stream"] != true {
		t.Fatalf("store=%v stream=%v", body["store"], body["stream"])
	}
}

func TestNoEffortLeavesTheModelsOwnThinkingDefault(t *testing.T) {
	c := &client{model: "m"}
	r := c.buildRequest(provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	if r.GenerationConfig.ThinkingLevel != "" || r.GenerationConfig.ThinkingSummaries != "" {
		t.Fatalf("generation_config = %+v", r.GenerationConfig)
	}
}

func TestImagesRideAsBase64ContentOnlyForAModelThatReadsThem(t *testing.T) {
	img := "data:image/png;base64,AAAA"
	msgs := []provider.Message{{Role: provider.RoleUser, Content: "look", Images: []string{img}}}
	with, _ := json.Marshal((&client{vision: true}).buildRequest(provider.Request{Messages: msgs}).Input)
	if !strings.Contains(string(with), `{"type":"image","data":"AAAA","mime_type":"image/png"}`) {
		t.Fatalf("vision input = %s", with)
	}
	without, _ := json.Marshal((&client{}).buildRequest(provider.Request{Messages: msgs}).Input)
	if strings.Contains(string(without), "image") {
		t.Fatalf("an image reached a model that does not read them: %s", without)
	}
}

func TestAFailedToolResultSaysSo(t *testing.T) {
	c := &client{}
	got, _ := json.Marshal(c.toolResult(provider.Message{Role: provider.RoleTool, ToolCallID: "a", Name: "n", Content: "boom", ToolFailure: &provider.ToolFailure{}}))
	if !strings.Contains(string(got), `"is_error":true`) {
		t.Fatalf("result = %s", got)
	}
}

func TestAnErrorEventBeforeAnyOutputCanBeTriedAgain(t *testing.T) {
	body := sse(textStream, `{"event_type":"error","error":{"code":"gateway_timeout","message":"upstream timed out"}}`)
	got := collect(t, newTestClient(t, serve(t, body, nil, nil, nil), nil), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	last := got[len(got)-1]
	if last.Type != provider.ChunkError || !provider.IsStreamInterrupted(last.Err) {
		t.Fatalf("last = %+v", last)
	}
	var payload *provider.StreamPayloadError
	if !errors.As(last.Err, &payload) || payload.Type != "gateway_timeout" {
		t.Fatalf("the error does not carry the code: %v", last.Err)
	}
}

func TestAFailedInteractionEndsAsAnError(t *testing.T) {
	body := sse(`{"event_type":"interaction.completed","interaction":{"status":"failed","errors":[{"code":"internal","message":"boom"}]}}`)
	got := collect(t, newTestClient(t, serve(t, body, nil, nil, nil), nil), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	if last := got[len(got)-1]; last.Type != provider.ChunkError {
		t.Fatalf("last = %+v", last)
	}
}

func TestAStreamThatEndsBeforeTheInteractionCompletesIsInterrupted(t *testing.T) {
	body := sse(textStream, `{"event_type":"step.start","index":0,"step":{"type":"model_output"}}`,
		`{"event_type":"step.delta","index":0,"delta":{"type":"text","text":"part"}}`)
	got := collect(t, newTestClient(t, serve(t, body, nil, nil, nil), nil), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	last := got[len(got)-1]
	if last.Type != provider.ChunkError || !provider.IsStreamInterrupted(last.Err) {
		t.Fatalf("last = %+v", last)
	}
}

func TestAnIncompleteInteractionIsALengthStop(t *testing.T) {
	body := sse(`{"event_type":"interaction.completed","interaction":{"status":"incomplete","usage":{"total_input_tokens":1,"total_output_tokens":1,"total_thought_tokens":5,"total_tokens":7}}}`)
	got := collect(t, newTestClient(t, serve(t, body, nil, nil, nil), nil), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	for _, c := range got {
		if c.Type == provider.ChunkUsage && c.Usage.FinishReason != "length" {
			t.Fatalf("finish = %q", c.Usage.FinishReason)
		}
	}
}

func TestARefusedKeyIsAnAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":403,"message":"API key not valid","status":"PERMISSION_DENIED"}}`)
	}))
	defer srv.Close()
	p := newTestClient(t, srv, map[string]any{"api_key_env": "GEMINI_API_KEY"})
	_, err := p.Stream(context.Background(), provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "x"}}})
	var auth *provider.AuthError
	if !errors.As(err, &auth) || auth.KeyEnv != "GEMINI_API_KEY" {
		t.Fatalf("err = %v", err)
	}
}

func TestAnEntryWithoutAModelIsRefused(t *testing.T) {
	if _, err := New(provider.Config{Name: "g"}); err == nil {
		t.Fatal("a provider with no model was built")
	}
}

func TestTheKindIsRegistered(t *testing.T) {
	p, err := provider.New("gemini", provider.Config{Model: "gemini-3.8-flash", APIKey: "k"})
	if err != nil || p.Name() != "gemini" {
		t.Fatalf("p=%v err=%v", p, err)
	}
}
