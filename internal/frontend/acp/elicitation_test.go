package acp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/event"
)

type answered struct {
	id      string
	answers []event.AskAnswer
}

func elicitSink(t *testing.T, caps string, reply string) (*updateSink, *fakeNotifier, chan answered) {
	t.Helper()
	var c ClientCapabilities
	if err := json.Unmarshal([]byte(caps), &c); err != nil {
		t.Fatal(err)
	}
	fn := &fakeNotifier{onReq: func(method string, _ any) (json.RawMessage, error) {
		if method != "elicitation/create" {
			t.Errorf("unexpected request %q", method)
		}
		return json.RawMessage(reply), nil
	}}
	sink := newUpdateSink(fn, "sess-1")
	sink.bindElicitation(c)
	got := make(chan answered, 1)
	sink.bindAnswer(func(id string, a []event.AskAnswer) error { got <- answered{id, a}; return nil })
	return sink, fn, got
}

func serverForm(multi bool) event.Ask {
	return event.Ask{ID: "ask-9", Origin: &event.AskOrigin{Kind: event.AskOriginMCP, Source: "tracker", Message: "Pick a project\x1b[2J"},
		Questions: []event.AskQuestion{
			{ID: "project", Header: "Project", Prompt: "Which one", Options: []event.AskOption{{Label: "alpha"}, {Label: "beta"}}, Multi: multi},
			{ID: "note", Header: "Note"},
		}}
}

func await(t *testing.T, ch chan answered) answered {
	t.Helper()
	select {
	case a := <-ch:
		return a
	case <-time.After(5 * time.Second):
		t.Fatal("the ask was never answered")
		return answered{}
	}
}

func TestAServerFormIsDeclinedWithoutAskingWhenTheClientAdvertisedNoMode(t *testing.T) {
	for _, caps := range []string{`{}`, `{"elicitation":null}`, `{"elicitation":{}}`, `{"elicitation":{"form":null,"url":null}}`} {
		sink, fn, got := elicitSink(t, caps, `{"action":"accept","content":{"project":"alpha"}}`)
		sink.requestAsk(t.Context(), serverForm(false))
		if a := await(t, got); a.answers != nil {
			t.Fatalf("%s: answered %+v", caps, a.answers)
		}
		if len(fn.reqSeen) != 0 {
			t.Fatalf("%s: a request was sent: %+v", caps, fn.reqSeen)
		}
	}
}

func TestAServerFormGoesThroughElicitationCreateAndSaysWhoIsAsking(t *testing.T) {
	sink, fn, got := elicitSink(t, `{"elicitation":{"form":{}}}`, `{"action":"accept","content":{"project":"beta","note":"hello"}}`)
	sink.requestAsk(t.Context(), serverForm(false))
	a := await(t, got)
	if a.id != "ask-9" || len(a.answers) != 2 || a.answers[0].Selected[0] != "beta" || a.answers[1].Selected[0] != "hello" {
		t.Fatalf("answers = %+v", a)
	}
	raw, _ := json.Marshal(fn.reqSeen[0].params)
	var p struct {
		SessionID, Mode, Message string
		RequestedSchema          struct {
			Type       string
			Properties map[string]struct {
				Type  string
				Enum  []string
				Title string
			}
		}
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.SessionID != "sess-1" || p.Mode != "form" || !strings.HasPrefix(p.Message, "MCP server tracker asks: Pick a project") || strings.ContainsRune(p.Message, 0x1b) {
		t.Fatalf("params = %+v", p)
	}
	if p.RequestedSchema.Type != "object" || p.RequestedSchema.Properties["project"].Type != "string" || len(p.RequestedSchema.Properties["project"].Enum) != 2 || p.RequestedSchema.Properties["note"].Type != "string" {
		t.Fatalf("schema = %+v", p.RequestedSchema)
	}
}

func TestADeclinedOrCancelledOrMalformedFormAnswersNothing(t *testing.T) {
	for _, reply := range []string{`{"action":"decline"}`, `{"action":"cancel","content":{"project":"alpha"}}`, `not json`, `{"action":"accept","content":{"project":"gamma"}}`,
		`{"action":"accept","content":{"project":["alpha","beta"]}}`, `{"action":"accept","content":{"project":42}}`} {
		sink, _, got := elicitSink(t, `{"elicitation":{"form":{}}}`, reply)
		sink.requestAsk(t.Context(), serverForm(false))
		if a := await(t, got); a.answers != nil {
			t.Fatalf("%s reached the server as %+v", reply, a.answers)
		}
	}
}

func TestAMultiChoiceTakesAnArrayAndChecksEveryValue(t *testing.T) {
	sink, _, got := elicitSink(t, `{"elicitation":{"form":{}}}`, `{"action":"accept","content":{"project":["alpha","beta"]}}`)
	sink.requestAsk(t.Context(), serverForm(true))
	if a := await(t, got); len(a.answers) != 1 || len(a.answers[0].Selected) != 2 {
		t.Fatalf("answers = %+v", a)
	}
	sink, _, got = elicitSink(t, `{"elicitation":{"form":{}}}`, `{"action":"accept","content":{"project":["alpha","zzz"]}}`)
	sink.requestAsk(t.Context(), serverForm(true))
	if a := await(t, got); a.answers != nil {
		t.Fatalf("a value outside the enum was accepted: %+v", a.answers)
	}
}

func serverURL() event.Ask {
	return event.Ask{ID: "ask-7", Origin: &event.AskOrigin{Kind: event.AskOriginMCP, Source: "tracker", Message: "Sign in", URL: "https://example.test/connect"},
		Questions: []event.AskQuestion{{ID: "mcp.url", Header: "External interaction"}}}
}

func TestAnExternalPageIsOfferedOnlyToAClientThatAdvertisedUrlMode(t *testing.T) {
	sink, fn, got := elicitSink(t, `{"elicitation":{"form":{}}}`, `{"action":"accept"}`)
	sink.requestAsk(t.Context(), serverURL())
	if a := await(t, got); a.answers != nil || len(fn.reqSeen) != 0 {
		t.Fatalf("a page was offered to a form-only client: %+v %+v", a, fn.reqSeen)
	}
	sink, fn, got = elicitSink(t, `{"elicitation":{"url":{}}}`, `{"action":"accept"}`)
	sink.requestAsk(t.Context(), serverURL())
	a := await(t, got)
	if len(a.answers) != 1 || a.answers[0].QuestionID != "mcp.url" || a.answers[0].Selected[0] != "accept" {
		t.Fatalf("answers = %+v", a)
	}
	raw, _ := json.Marshal(fn.reqSeen[0].params)
	var p map[string]any
	_ = json.Unmarshal(raw, &p)
	if p["mode"] != "url" || p["url"] != "https://example.test/connect" || p["elicitationId"] != "elicit-ask-7" || p["sessionId"] != "sess-1" {
		t.Fatalf("params = %v", p)
	}
}

func TestAFormIsNeverOfferedForAnInteractionThatNeedsAPage(t *testing.T) {
	sink, fn, got := elicitSink(t, `{"elicitation":{"form":{}}}`, `{"action":"accept"}`)
	sink.requestAsk(t.Context(), serverURL())
	if a := await(t, got); a.answers != nil || len(fn.reqSeen) != 0 {
		t.Fatalf("fell back to a form: %+v %+v", a, fn.reqSeen)
	}
}

func TestUrlModeCancelIsDistinctFromDecline(t *testing.T) {
	sink, _, got := elicitSink(t, `{"elicitation":{"url":{}}}`, `{"action":"cancel"}`)
	sink.requestAsk(t.Context(), serverURL())
	if a := await(t, got); len(a.answers) != 1 || a.answers[0].Selected[0] != "cancel" {
		t.Fatalf("answers = %+v", a)
	}
	sink, _, got = elicitSink(t, `{"elicitation":{"url":{}}}`, `{"action":"decline"}`)
	sink.requestAsk(t.Context(), serverURL())
	if a := await(t, got); a.answers != nil {
		t.Fatalf("decline answered %+v", a.answers)
	}
}
