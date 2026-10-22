package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func deepseekServer(t *testing.T, status int, body string, seen *struct {
	Req  *http.Request
	Body map[string]any
}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			seen.Req = r.Clone(context.Background())
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &seen.Body)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestDeepSeekAPISendsTheDocumentedRequestAndMapsResults(t *testing.T) {
	var seen struct {
		Req  *http.Request
		Body map[string]any
	}
	srv := deepseekServer(t, 200, `{"results":[{"title":"A","url":"https://a.example","snippet":"short","summary":"long","content":"","rerank_score":2.1}]}`, &seen)
	defer srv.Close()
	b := NewDeepSeekAPI(srv.URL, func() string { return "sk-test" }, srv.Client())
	hits, err := b.Search(context.Background(), "golang context")
	if err != nil {
		t.Fatal(err)
	}
	if seen.Req.Method != http.MethodPost || seen.Req.URL.Path != "/v1/tools/search" || seen.Req.Header.Get("Authorization") != "Bearer sk-test" {
		t.Fatalf("request = %s %s auth=%q", seen.Req.Method, seen.Req.URL.Path, seen.Req.Header.Get("Authorization"))
	}
	if seen.Body["query"] != "golang context" || seen.Body["count"] != float64(10) {
		t.Fatalf("body = %v", seen.Body)
	}
	if len(hits) != 1 || hits[0].Title != "A" || hits[0].URL != "https://a.example" || hits[0].Snippet != "short" {
		t.Fatalf("hits = %+v", hits)
	}
	if !b.Paid() || b.Name() != "deepseek_api" {
		t.Fatal("the DeepSeek API backend is paid and named deepseek_api")
	}
}

// The documented query limit is 128 characters; a longer query is cut to it
// rather than refused, and by characters, not bytes.
func TestDeepSeekAPICutsTheQueryToItsDocumentedLength(t *testing.T) {
	var seen struct {
		Req  *http.Request
		Body map[string]any
	}
	srv := deepseekServer(t, 200, `{"results":[{"title":"A","url":"https://a.example","snippet":"s"}]}`, &seen)
	defer srv.Close()
	b := NewDeepSeekAPI(srv.URL, func() string { return "k" }, srv.Client())
	if _, err := b.Search(context.Background(), strings.Repeat("搜", 300)); err != nil {
		t.Fatal(err)
	}
	if got := []rune(seen.Body["query"].(string)); len(got) != 128 {
		t.Fatalf("query is %d characters, want 128", len(got))
	}
}

// Each documented status is its own identity, owned by the backend that saw it.
func TestDeepSeekAPIStatusesCarryTypedCodes(t *testing.T) {
	cases := map[int]string{
		400: CodeSearchBadRequest,
		401: CodeSearchUnauthorized,
		402: CodeSearchInsufficientBalance,
		408: CodeSearchUnavailable,
		413: CodeSearchBadRequest,
		429: CodeSearchRateLimited,
		500: CodeSearchUnavailable,
		504: CodeSearchUnavailable,
	}
	for status, code := range cases {
		srv := deepseekServer(t, status, `{"error":{"message":"x"}}`, nil)
		b := NewDeepSeekAPI(srv.URL, func() string { return "k" }, srv.Client())
		_, err := b.Search(context.Background(), "q")
		srv.Close()
		var be *BackendError
		if !errors.As(err, &be) || be.Kind != FailStatus || be.Status != status || be.Code != code {
			t.Fatalf("HTTP %d: err = %+v, want status failure with code %s", status, err, code)
		}
	}
}

// Only a 200 with results is billable, so an empty answer is a failure the
// chain moves past, and nothing is reported as billed for it.
func TestDeepSeekAPIEmptyAnswerIsAFailureNotACharge(t *testing.T) {
	srv := deepseekServer(t, 200, `{"results":[]}`, nil)
	defer srv.Close()
	b := NewDeepSeekAPI(srv.URL, func() string { return "k" }, srv.Client())
	_, err := b.Search(context.Background(), "q")
	var be *BackendError
	if !errors.As(err, &be) || be.Kind != FailEmpty {
		t.Fatalf("err = %v, want FailEmpty", err)
	}
}

func TestDeepSeekAPIWithoutAKeyDoesNotCallOut(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	b := NewDeepSeekAPI(srv.URL, func() string { return "" }, srv.Client())
	_, err := b.Search(context.Background(), "q")
	var be *BackendError
	if !errors.As(err, &be) || be.Code != CodeSearchUnauthorized || called {
		t.Fatalf("err = %v called=%v, want an unauthorized failure and no request", err, called)
	}
}

// The chain reports a charge only for a paid backend that answered.
func TestChainReportsBilledRequestsOnlyForAnsweringPaidBackends(t *testing.T) {
	free := &fakeBackend{name: "searxng", hits: []Hit{{Title: "x"}}}
	var billed []string
	if _, err := run(t, &Chain{Backends: []Backend{free}, OnBilled: func(name string) { billed = append(billed, name) }}); err != nil || len(billed) != 0 {
		t.Fatalf("a free answer was billed: %v %v", billed, err)
	}
	paid := &fakeBackend{name: "deepseek_api", paid: true, err: &BackendError{Kind: FailEmpty}}
	_, _ = run(t, &Chain{Backends: []Backend{paid}, Consent: func(context.Context, Backend, string) bool { return true }, OnBilled: func(name string) { billed = append(billed, name) }})
	if len(billed) != 0 {
		t.Fatalf("an empty paid answer was billed: %v", billed)
	}
	paid = &fakeBackend{name: "deepseek_api", paid: true, hits: []Hit{{Title: "p"}}}
	if _, err := run(t, &Chain{Backends: []Backend{paid}, Consent: func(context.Context, Backend, string) bool { return true }, OnBilled: func(name string) { billed = append(billed, name) }}); err != nil || len(billed) != 1 || billed[0] != "deepseek_api" {
		t.Fatalf("billed = %v err=%v", billed, err)
	}
}

// When everything failed the model is told each backend's identity, so a
// balance problem is not mistaken for an empty result.
func TestChainRefusalNamesEachBackendsFailureIdentity(t *testing.T) {
	free := &fakeBackend{name: "searxng", err: &BackendError{Backend: "searxng", Kind: FailEmpty}}
	paid := &fakeBackend{name: "deepseek_api", paid: true, err: &BackendError{Backend: "deepseek_api", Kind: FailStatus, Status: 402, Code: CodeSearchInsufficientBalance}}
	_, err := run(t, &Chain{Backends: []Backend{free, paid}, Consent: func(context.Context, Backend, string) bool { return true }})
	if err == nil || !strings.Contains(err.Error(), "deepseek_api: "+CodeSearchInsufficientBalance) || !strings.Contains(err.Error(), "searxng: empty") {
		t.Fatalf("err = %v", err)
	}
}
