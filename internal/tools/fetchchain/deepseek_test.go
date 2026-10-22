package fetchchain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/tools/websearch"
)

func fetchServer(t *testing.T, status int, body string, seen *struct {
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

func TestDeepSeekFetchSendsTheDocumentedRequestAndReturnsMarkdown(t *testing.T) {
	var seen struct {
		Req  *http.Request
		Body map[string]any
	}
	srv := fetchServer(t, 200, `{"title":"Docs","content":"# Heading\n\nbody"}`, &seen)
	defer srv.Close()
	b := NewDeepSeekFetch(srv.URL, func() string { return "sk-test" }, srv.Client())
	page, err := b.Fetch(context.Background(), "https://example.com/docs")
	if err != nil {
		t.Fatal(err)
	}
	if seen.Req.Method != http.MethodPost || seen.Req.URL.Path != "/v1/tools/fetch" || seen.Req.Header.Get("Authorization") != "Bearer sk-test" || seen.Body["url"] != "https://example.com/docs" {
		t.Fatalf("request = %s %s auth=%q body=%v", seen.Req.Method, seen.Req.URL.Path, seen.Req.Header.Get("Authorization"), seen.Body)
	}
	if page.Title != "Docs" || !strings.Contains(page.Text, "body") || !b.Paid() || b.Name() != "deepseek_api" {
		t.Fatalf("page=%+v paid=%v name=%s", page, b.Paid(), b.Name())
	}
}

func TestDeepSeekFetchStatusesCarryTypedCodes(t *testing.T) {
	cases := map[int]string{
		400: CodeFetchBadRequest,
		401: CodeFetchUnauthorized,
		402: CodeFetchInsufficientBalance,
		403: CodeFetchRefusedByPolicy,
		404: CodeFetchNoContent,
		408: CodeFetchUnavailable,
		413: CodeFetchBadRequest,
		429: CodeFetchRateLimited,
		500: CodeFetchUnavailable,
		504: CodeFetchUnavailable,
	}
	for status, code := range cases {
		srv := fetchServer(t, status, `{"error":{"message":"x"}}`, nil)
		b := NewDeepSeekFetch(srv.URL, func() string { return "k" }, srv.Client())
		_, err := b.Fetch(context.Background(), "https://example.com")
		srv.Close()
		var be *websearch.BackendError
		if !errors.As(err, &be) || be.Kind != websearch.FailStatus || be.Status != status || be.Code != code {
			t.Fatalf("HTTP %d: err = %+v, want status failure with code %s", status, err, code)
		}
	}
}

// Only a 200 with page content is billed, so a 200 with none is a failure.
func TestDeepSeekFetchEmptyContentIsAFailureNotACharge(t *testing.T) {
	srv := fetchServer(t, 200, `{"title":"","content":""}`, nil)
	defer srv.Close()
	b := NewDeepSeekFetch(srv.URL, func() string { return "k" }, srv.Client())
	_, err := b.Fetch(context.Background(), "https://example.com")
	var be *websearch.BackendError
	if !errors.As(err, &be) || be.Kind != websearch.FailEmpty {
		t.Fatalf("err = %v, want FailEmpty", err)
	}
}

func TestDeepSeekFetchWithoutAKeyDoesNotCallOut(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	b := NewDeepSeekFetch(srv.URL, func() string { return "" }, srv.Client())
	_, err := b.Fetch(context.Background(), "https://example.com")
	var be *websearch.BackendError
	if !errors.As(err, &be) || be.Code != CodeFetchUnauthorized || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

// The page is third-party text of unbounded size; what reaches the model is cut
// on a character boundary.
func TestDeepSeekFetchCapsTheContentOnACharacterBoundary(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"title": "t", "content": strings.Repeat("页", deepseekFetchMaxContent)})
	srv := fetchServer(t, 200, string(body), nil)
	defer srv.Close()
	b := NewDeepSeekFetch(srv.URL, func() string { return "k" }, srv.Client())
	page, err := b.Fetch(context.Background(), "https://example.com")
	if err != nil || len(page.Text) > deepseekFetchMaxContent+len("\n\n(truncated)") {
		t.Fatalf("len=%d err=%v", len(page.Text), err)
	}
	for _, r := range page.Text {
		if r == '�' {
			t.Fatal("the cut landed inside a character")
		}
	}
}
