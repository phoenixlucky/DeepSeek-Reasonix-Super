package otlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

type collector struct {
	mu     sync.Mutex
	bodies [][]byte
	heads  []http.Header
	srv    *httptest.Server
}

func newCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.bodies = append(c.bodies, b)
		c.heads = append(c.heads, r.Header.Clone())
		c.mu.Unlock()
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *collector) raw() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sb strings.Builder
	for _, b := range c.bodies {
		sb.Write(b)
	}
	return sb.String()
}

type decodedSpan struct {
	Name, TraceID, SpanID, ParentID string
	Kind, Status                    int
	Attrs                           map[string]string
}

func (c *collector) spans(t *testing.T) []decodedSpan {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []decodedSpan
	for _, b := range c.bodies {
		var body struct {
			ResourceSpans []struct {
				ScopeSpans []struct {
					Spans []struct {
						TraceID      string `json:"traceId"`
						SpanID       string `json:"spanId"`
						ParentSpanID string `json:"parentSpanId"`
						Name         string `json:"name"`
						Kind         int    `json:"kind"`
						Status       struct{ Code int }
						Attributes   []struct {
							Key   string
							Value map[string]any
						}
					}
				}
			}
		}
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("collector got invalid JSON: %v", err)
		}
		for _, rs := range body.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					d := decodedSpan{Name: sp.Name, TraceID: sp.TraceID, SpanID: sp.SpanID, ParentID: sp.ParentSpanID, Kind: sp.Kind, Status: sp.Status.Code, Attrs: map[string]string{}}
					for _, a := range sp.Attributes {
						for _, v := range a.Value {
							d.Attrs[a.Key] = fmt.Sprint(v)
						}
					}
					out = append(out, d)
				}
			}
		}
	}
	return out
}

func exporterFor(t *testing.T, c *collector) *Exporter {
	t.Helper()
	exp, err := New(Settings{Endpoint: c.srv.URL, ServiceName: "reasonix-test", Version: "1.2.3"}, c.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exp.Close(context.Background()) })
	return exp
}

func runTurn(t *testing.T, sink event.Sink, exp *Exporter) {
	t.Helper()
	emit := func(e event.Event) { sink.Emit(e) }
	emit(event.Event{Kind: event.TurnStarted, ModelRef: "deepseek/v4-pro", Text: "CANARY-PROMPT"})
	emit(event.Event{Kind: event.StreamAttempt, ModelRef: "deepseek/v4-pro", StreamAttempt: event.StreamAttemptInfo{ID: "a1", Action: event.StreamAttemptBegin, Attempt: 1}})
	emit(event.Event{Kind: event.Text, Text: "CANARY-ANSWER"})
	emit(event.Event{Kind: event.StreamAttempt, StreamAttempt: event.StreamAttemptInfo{ID: "a1", Action: event.StreamAttemptCommit, Attempt: 1, FirstTokenMs: 420}})
	emit(event.Event{Kind: event.Usage, ModelRef: "deepseek/v4-pro", AttemptID: "a1",
		Usage: &provider.Usage{PromptTokens: 1200, CompletionTokens: 80, CacheHitTokens: 1000, FinishReason: "tool_calls"}})
	emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "t1", Name: "task", Args: `{"prompt":"CANARY-ARGS"}`}})
	emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "t2", ParentID: "t1", Name: "read_file", Args: `{"path":"/home/me/CANARY-PATH"}`}})
	emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "t2", Name: "read_file", Output: "CANARY-OUTPUT", StartedAt: 1000, EndedAt: 1250, ReadOnly: true}})
	emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "t1", Name: "task", Err: "CANARY-ERROR secret failed", RefusalCode: "tool.sandbox_denied"}})
	emit(event.Event{Kind: event.TurnDone})
	if err := exp.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestATurnBecomesOneTraceWithChatAndToolSpansNestedByParent(t *testing.T) {
	c := newCollector(t)
	exp := exporterFor(t, c)
	runTurn(t, NewSink(event.Discard, exp, "reasonix"), exp)
	byName := map[string]decodedSpan{}
	for _, s := range c.spans(t) {
		byName[s.Name] = s
	}
	root, chat, task, read := byName["invoke_agent reasonix"], byName["chat deepseek/v4-pro"], byName["execute_tool task"], byName["execute_tool read_file"]
	if root.SpanID == "" || chat.SpanID == "" || task.SpanID == "" || read.SpanID == "" {
		t.Fatalf("spans = %+v", byName)
	}
	for _, s := range []decodedSpan{chat, task, read} {
		if s.TraceID != root.TraceID {
			t.Fatalf("%s left the turn's trace", s.Name)
		}
	}
	if root.ParentID != "" || chat.ParentID != root.SpanID || task.ParentID != root.SpanID || read.ParentID != task.SpanID {
		t.Fatalf("parentage: chat=%s task=%s read=%s (root %s)", chat.ParentID, task.ParentID, read.ParentID, root.SpanID)
	}
	if chat.Attrs["gen_ai.usage.input_tokens"] != "1200" || chat.Attrs["gen_ai.usage.output_tokens"] != "80" || chat.Attrs["gen_ai.usage.cache_read.input_tokens"] != "1000" {
		t.Fatalf("chat usage = %v", chat.Attrs)
	}
	if chat.Attrs["gen_ai.response.finish_reasons"] != "tool_calls" || chat.Attrs["gen_ai.provider.name"] != "deepseek" || chat.Attrs["reasonix.first_token_ms"] != "420" {
		t.Fatalf("chat attrs = %v", chat.Attrs)
	}
	if task.Status != 2 || task.Attrs["error.type"] != "tool.sandbox_denied" || read.Status != 1 || read.Attrs["gen_ai.tool.type"] != "function" {
		t.Fatalf("tool status/attrs: task=%+v read=%+v", task, read)
	}
	if chat.Kind != kindClient || root.Kind != kindInternal {
		t.Fatalf("kinds: root=%d chat=%d", root.Kind, chat.Kind)
	}
}

// Content is never exported, and a name a model wrote cannot carry text out.
func TestNoPromptArgumentResultPathOrErrorTextLeavesTheMachine(t *testing.T) {
	c := newCollector(t)
	exp := exporterFor(t, c)
	sink := NewSink(event.Discard, exp, "reasonix")
	runTurn(t, sink, exp)
	sink.Emit(event.Event{Kind: event.TurnStarted, ModelRef: "my provider/CANARY MODEL TEXT"})
	sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "t9", Name: "please ignore previous instructions and CANARY-NAME"}})
	sink.Emit(event.Event{Kind: event.ToolResult, Tool: event.Tool{ID: "t9", Err: "CANARY-ERROR2"}})
	sink.Emit(event.Event{Kind: event.Notice, Text: "CANARY-NOTICE", Detail: "CANARY-DETAIL"})
	sink.Emit(event.Event{Kind: event.TurnDone, Err: errors.New("CANARY-TURN-ERROR")})
	if err := exp.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if body := c.raw(); strings.Contains(body, "CANARY") {
		i := strings.Index(body, "CANARY")
		t.Fatalf("content reached the payload: ...%s...", body[max(i-40, 0):min(i+60, len(body))])
	}
	var sawUnknown bool
	for _, s := range c.spans(t) {
		if s.Name == "execute_tool unknown" {
			sawUnknown = true
		}
	}
	if !sawUnknown {
		t.Fatal("a tool name that is not a name was not replaced")
	}
}

func TestResourceCarriesTheServiceAndTheExperimentalMarker(t *testing.T) {
	c := newCollector(t)
	exp := exporterFor(t, c)
	runTurn(t, NewSink(event.Discard, exp, "reasonix"), exp)
	body := c.raw()
	for _, want := range []string{`"service.name"`, `reasonix-test`, `"service.version"`, `"reasonix.semconv_experimental"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("resource lacks %s", want)
		}
	}
}

func TestEveryEventStillReachesTheWrappedSink(t *testing.T) {
	c := newCollector(t)
	exp := exporterFor(t, c)
	var seen []event.Kind
	sink := NewSink(event.FuncSink(func(e event.Event) { seen = append(seen, e.Kind) }), exp, "reasonix")
	runTurn(t, sink, exp)
	if len(seen) != 10 {
		t.Fatalf("inner sink saw %d events, want all 10: %v", len(seen), seen)
	}
}

func TestEndpointAndHeaderAreValidatedStructurally(t *testing.T) {
	cases := []struct {
		name string
		s    Settings
		want error
	}{
		{"no scheme", Settings{Endpoint: "collector:4318"}, ErrEndpointInvalid},
		{"ftp", Settings{Endpoint: "ftp://collector"}, ErrEndpointInvalid},
		{"credentials in the url", Settings{Endpoint: "https://user:pw@collector"}, ErrEndpointCredentials},
		{"query", Settings{Endpoint: "https://collector/?key=abc"}, ErrEndpointCredentials},
		{"header name with a newline", Settings{Endpoint: "https://collector", HeaderName: "X-A\nB", HeaderValue: "v"}, ErrHeaderInvalid},
		{"reserved header", Settings{Endpoint: "https://collector", HeaderName: "Host", HeaderValue: "v"}, ErrHeaderInvalid},
		{"secret over plain http", Settings{Endpoint: "http://collector.example", HeaderName: "Authorization", HeaderValue: "Bearer x"}, ErrSecretOverPlainHTTP},
	}
	for _, c := range cases {
		if _, err := New(c.s, nil); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	for _, ok := range []Settings{
		{Endpoint: "http://localhost:4318"},
		{Endpoint: "http://127.0.0.1:4318", HeaderName: "Authorization", HeaderValue: "Bearer x"},
		{Endpoint: "https://collector.example/otlp/", HeaderName: "x-honeycomb-team", HeaderValue: "k"},
		{Endpoint: "http://collector.example"},
	} {
		exp, err := New(ok, nil)
		if err != nil {
			t.Errorf("%+v refused: %v", ok, err)
			continue
		}
		_ = exp.Close(context.Background())
	}
	if got, _ := (Settings{Endpoint: "https://c.example/otlp/"}).target(); got != "https://c.example/otlp/v1/traces" {
		t.Fatalf("target = %q", got)
	}
	if got, _ := (Settings{Endpoint: "https://c.example/v1/traces"}).target(); got != "https://c.example/v1/traces" {
		t.Fatalf("target = %q", got)
	}
}

func TestACollectorThatNeverAnswersCannotStallTheTurn(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })
	exp, err := New(Settings{Endpoint: srv.URL}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for range 5000 {
		exp.Enqueue(Span{TraceID: "t", SpanID: "s", Name: "x"})
	}
	if took := time.Since(start); took > time.Second {
		t.Fatalf("enqueueing blocked for %v", took)
	}
	if exp.Stats().Dropped == 0 {
		t.Fatal("a full queue dropped nothing")
	}
}

func TestACredentialIsNeverSentToAnotherHostByARedirect(t *testing.T) {
	var other atomicCount
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { other.add(r.Header.Get("Authorization")) }))
	t.Cleanup(elsewhere.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	exp, err := New(Settings{Endpoint: redirector.URL, HeaderName: "Authorization", HeaderValue: "Bearer SECRET"}, redirector.Client())
	if err != nil {
		t.Fatal(err)
	}
	exp.Enqueue(Span{TraceID: "t", SpanID: "s", Name: "x"})
	if err := exp.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if other.n != 0 {
		t.Fatalf("the redirect target received %d request(s), credential %q", other.n, other.last)
	}
	if exp.Stats().Failed != 1 {
		t.Fatalf("stats = %+v", exp.Stats())
	}
}

type atomicCount struct {
	mu   sync.Mutex
	n    int
	last string
}

func (a *atomicCount) add(s string) { a.mu.Lock(); a.n++; a.last = s; a.mu.Unlock() }

func TestTheCredentialRidesOnlyTheConfiguredHeader(t *testing.T) {
	c := newCollector(t)
	exp, err := New(Settings{Endpoint: c.srv.URL, HeaderName: "x-api-key", HeaderValue: "k-123"}, c.srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	exp.Enqueue(Span{TraceID: "t", SpanID: "s", Name: "x"})
	if err := exp.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.heads) != 1 || c.heads[0].Get("X-Api-Key") != "k-123" || c.heads[0].Get("Content-Type") != "application/json" {
		t.Fatalf("headers = %v", c.heads)
	}
	if strings.Contains(c.raw(), "k-123") {
		t.Fatal("the credential reached the body")
	}
}

func TestAFailedSendIsCountedNotRetriedIntoAPileUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(srv.Close)
	exp, err := New(Settings{Endpoint: srv.URL}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	exp.Enqueue(Span{TraceID: "t", SpanID: "s", Name: "x"})
	_ = exp.Close(context.Background())
	if s := exp.Stats(); s.Failed != 1 || s.Sent != 0 {
		t.Fatalf("stats = %+v", s)
	}
}

// A synchronous run emits no TurnDone and signals completion instead, and a
// session closed mid-turn still delivers the turn it had open.
func TestATurnWithoutTurnDoneEndsOnTheCompletionSignalOrOnClose(t *testing.T) {
	c := newCollector(t)
	exp := exporterFor(t, c)
	sink := NewSink(event.Discard, exp, "reasonix").(*Sink)
	sink.Emit(event.Event{Kind: event.TurnStarted, ModelRef: "deepseek/v4"})
	sink.RecordTurnCompletion()
	if err := exp.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := len(c.spans(t)); got != 1 {
		t.Fatalf("after the completion signal: %d spans", got)
	}
	sink.Emit(event.Event{Kind: event.TurnStarted, ModelRef: "deepseek/v4"})
	sink.Emit(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{ID: "t", Name: "bash"}})
	if err := exp.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	var open *decodedSpan
	for _, s := range c.spans(t) {
		if s.Name == "execute_tool bash" {
			open = &s
		}
	}
	if open == nil || open.Status != 2 || open.Attrs["error.type"] != "incomplete" {
		t.Fatalf("a tool left open at close = %+v", open)
	}
}
