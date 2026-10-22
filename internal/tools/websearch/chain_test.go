package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"reasonix/internal/contract/tool"
)

type fakeBackend struct {
	name  string
	paid  bool
	hits  []Hit
	err   error
	calls int
}

func (f *fakeBackend) Name() string { return f.name }
func (f *fakeBackend) Paid() bool   { return f.paid }
func (f *fakeBackend) Search(context.Context, string) ([]Hit, error) {
	f.calls++
	return f.hits, f.err
}

func run(t *testing.T, c *Chain) (string, error) {
	t.Helper()
	return c.Execute(context.Background(), json.RawMessage(`{"query":"golang context"}`))
}

func TestChainAnswersFromTheFirstBackendThatHasResults(t *testing.T) {
	free := &fakeBackend{name: "searxng", hits: []Hit{{Title: "T", URL: "https://example.com", Snippet: "S"}}}
	later := &fakeBackend{name: "later", hits: []Hit{{Title: "X"}}}
	out, err := run(t, &Chain{Backends: []Backend{free, later}})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Backend string `json:"backend"`
		Paid    bool   `json:"paid"`
		Results []Hit  `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Backend != "searxng" || got.Paid || len(got.Results) != 1 || got.Results[0].URL != "https://example.com" {
		t.Fatalf("result = %+v", got)
	}
	if later.calls != 0 {
		t.Fatal("a later backend was called although the first answered")
	}
}

// Every way a free backend can fail to answer falls through to the next, and
// each fall-through is reported with its structural cause.
func TestChainFallsThroughOnEveryStructuralFailure(t *testing.T) {
	cases := map[string]error{
		"transport": &BackendError{Backend: "a", Kind: FailTransport, Err: errors.New("dial")},
		"status":    &BackendError{Backend: "a", Kind: FailStatus, Status: 503},
		"decode":    &BackendError{Backend: "a", Kind: FailDecode, Err: errors.New("json")},
		"empty":     &BackendError{Backend: "a", Kind: FailEmpty},
		"untyped":   errors.New("anything else"),
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			first := &fakeBackend{name: "a", err: failure}
			second := &fakeBackend{name: "b", hits: []Hit{{Title: "ok", URL: "https://example.org"}}}
			var falls []Fallback
			out, err := run(t, &Chain{Backends: []Backend{first, second}, OnFallback: func(f Fallback) { falls = append(falls, f) }})
			if err != nil || !strings.Contains(out, `"backend":"b"`) {
				t.Fatalf("out=%q err=%v", out, err)
			}
			if len(falls) != 1 || falls[0].Backend != "a" || falls[0].Next != "b" {
				t.Fatalf("fallbacks = %+v", falls)
			}
			wantKind := FailKind(name)
			if name == "untyped" {
				wantKind = FailTransport
			}
			if falls[0].Kind != wantKind {
				t.Fatalf("kind = %q, want %q", falls[0].Kind, wantKind)
			}
		})
	}
}

func TestChainDoesNotMistakeACancelledSearchForAFailedBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeBackend{name: "a", err: context.Canceled}
	second := &fakeBackend{name: "b", hits: []Hit{{Title: "x"}}}
	cancel()
	_, err := (&Chain{Backends: []Backend{first, second}}).Execute(ctx, json.RawMessage(`{"query":"q"}`))
	if !errors.Is(err, context.Canceled) || second.calls != 0 {
		t.Fatalf("err=%v second.calls=%d, want a cancellation and no fall-through", err, second.calls)
	}
}

// A paid backend is only reached after the free ones failed, and only with the
// user's consent; without it the model gets a typed refusal, not a charge.
func TestChainNeverCallsAPaidBackendWithoutConsent(t *testing.T) {
	free := &fakeBackend{name: "searxng", err: &BackendError{Kind: FailEmpty}}
	paid := &fakeBackend{name: "deepseek_api", paid: true, hits: []Hit{{Title: "p"}}}
	_, err := run(t, &Chain{Backends: []Backend{free, paid}})
	var ref tool.Refusal
	if !errors.As(err, &ref) || ref.Code != CodeFreeFailedPaidNeedsConsent {
		t.Fatalf("err = %v, want refusal %s", err, CodeFreeFailedPaidNeedsConsent)
	}
	if paid.calls != 0 {
		t.Fatal("the paid backend was called without consent")
	}
}

func TestChainCallsAPaidBackendOnceConsentIsGivenAndMarksTheResult(t *testing.T) {
	free := &fakeBackend{name: "searxng", err: &BackendError{Kind: FailStatus, Status: 429}}
	paid := &fakeBackend{name: "deepseek_api", paid: true, hits: []Hit{{Title: "p", URL: "https://example.net"}}}
	var asked []string
	out, err := run(t, &Chain{Backends: []Backend{free, paid}, Consent: func(_ context.Context, b Backend, _ string) bool {
		asked = append(asked, b.Name())
		return true
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "deepseek_api" || !strings.Contains(out, `"paid":true`) || !strings.Contains(out, `"backend":"deepseek_api"`) {
		t.Fatalf("asked=%v out=%s", asked, out)
	}
}

func TestChainWithEveryBackendFailedSaysSoWithACode(t *testing.T) {
	a := &fakeBackend{name: "a", err: &BackendError{Kind: FailEmpty}}
	_, err := run(t, &Chain{Backends: []Backend{a}})
	var ref tool.Refusal
	if !errors.As(err, &ref) || ref.Code != CodeAllBackendsFailed {
		t.Fatalf("err = %v, want refusal %s", err, CodeAllBackendsFailed)
	}
}

func TestChainRejectsAnEmptyQuery(t *testing.T) {
	_, err := (&Chain{Backends: []Backend{&fakeBackend{name: "a"}}}).Execute(context.Background(), json.RawMessage(`{"query":"  "}`))
	if !errors.Is(err, ErrQuery) {
		t.Fatalf("err = %v", err)
	}
}

func searxServer(t *testing.T, status int, body string, seen *http.Request) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = *r
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestSearxngSendsTheQueryAsJSONAndMapsResults(t *testing.T) {
	var seen http.Request
	srv := searxServer(t, 200, `{"results":[{"title":"A","url":"https://a.example","content":"about a","engine":"google"},{"title":"B","url":"https://b.example","content":"","engine":"brave"}],"unresponsive_engines":[["brave","CAPTCHA"]]}`, &seen)
	defer srv.Close()
	b, err := NewSearxng(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	hits, err := b.Search(context.Background(), "go & context")
	if err != nil {
		t.Fatal(err)
	}
	if seen.URL.Path != "/search" || seen.URL.Query().Get("q") != "go & context" || seen.URL.Query().Get("format") != "json" {
		t.Fatalf("request = %s", seen.URL.String())
	}
	if len(hits) != 2 || hits[0].Title != "A" || hits[0].Snippet != "about a" || hits[1].URL != "https://b.example" {
		t.Fatalf("hits = %+v", hits)
	}
	if b.Paid() || b.Name() != "searxng" {
		t.Fatal("searxng must be a free backend named searxng")
	}
}

// Whether the free backend answered is read off the response, not its prose.
func TestSearxngFailuresCarryTheirStructuralKind(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		kind   FailKind
	}{
		{"non-200", 429, `{"results":[{"title":"x","url":"https://x"}]}`, FailStatus},
		{"bad json", 200, `<html>`, FailDecode},
		{"no results", 200, `{"results":[]}`, FailEmpty},
		{"engines throttled and nothing left", 200, `{"results":[],"unresponsive_engines":[["google","Suspended: too many requests"],["brave","CAPTCHA"]]}`, FailEmpty},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := searxServer(t, c.status, c.body, nil)
			defer srv.Close()
			b, _ := NewSearxng(srv.URL, srv.Client())
			_, err := b.Search(context.Background(), "q")
			var be *BackendError
			if !errors.As(err, &be) || be.Kind != c.kind {
				t.Fatalf("err = %v, want kind %s", err, c.kind)
			}
		})
	}
	t.Run("transport", func(t *testing.T) {
		srv := searxServer(t, 200, `{}`, nil)
		url := srv.URL
		srv.Close()
		b, _ := NewSearxng(url, http.DefaultClient)
		_, err := b.Search(context.Background(), "q")
		var be *BackendError
		if !errors.As(err, &be) || be.Kind != FailTransport {
			t.Fatalf("err = %v, want transport", err)
		}
	})
}

func TestSearxngSurvivesOnOneEngineWhenTheOthersAreThrottled(t *testing.T) {
	srv := searxServer(t, 200, `{"results":[{"title":"A","url":"https://a.example","content":"c","engine":"google"}],"unresponsive_engines":[["brave","CAPTCHA"],["duckduckgo","Suspended: too many requests"]]}`, nil)
	defer srv.Close()
	b, _ := NewSearxng(srv.URL, srv.Client())
	hits, err := b.Search(context.Background(), "q")
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%v err=%v, want success with the one engine left", hits, err)
	}
}

func TestSearxngRefusesAnUnusableURL(t *testing.T) {
	for _, raw := range []string{"", "ftp://x", "http://", "http://u:p@host", "http://h/?a=b", "not a url"} {
		if _, err := NewSearxng(raw, http.DefaultClient); err == nil {
			t.Fatalf("%q accepted", raw)
		}
	}
}

// A snippet cut at the byte limit never ends inside a multi-byte character.
func TestSearxngSnippetsAreCutOnACharacterBoundary(t *testing.T) {
	long := strings.Repeat("搜", searxngMaxSnippet)
	body, _ := json.Marshal(map[string]any{"results": []map[string]string{{"title": "t", "url": "https://a.example", "content": long}}})
	srv := searxServer(t, 200, string(body), nil)
	defer srv.Close()
	b, _ := NewSearxng(srv.URL, srv.Client())
	hits, err := b.Search(context.Background(), "q")
	if err != nil || len(hits) != 1 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
	if !utf8.ValidString(hits[0].Snippet) || len(hits[0].Snippet) > searxngMaxSnippet {
		t.Fatalf("snippet is %d bytes, valid UTF-8: %v", len(hits[0].Snippet), utf8.ValidString(hits[0].Snippet))
	}
}
