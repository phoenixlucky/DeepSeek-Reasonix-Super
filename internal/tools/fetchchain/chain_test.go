package fetchchain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/builtin"
	"reasonix/internal/tools/websearch"
)

type fakeBackend struct {
	name  string
	paid  bool
	page  Page
	err   error
	calls int
}

func (f *fakeBackend) Name() string { return f.name }
func (f *fakeBackend) Paid() bool   { return f.paid }
func (f *fakeBackend) Fetch(context.Context, string) (Page, error) {
	f.calls++
	return f.page, f.err
}

func run(t *testing.T, c *Chain, url string) (string, error) {
	t.Helper()
	return c.Execute(context.Background(), json.RawMessage(fmt.Sprintf(`{"url":%q}`, url)))
}

func TestChainReturnsTheFirstBackendsPageUnchanged(t *testing.T) {
	local := &fakeBackend{name: "local", page: Page{Rendered: "status 200 OK · text/plain · 5 bytes\n\nhello", Text: "hello", Status: 200}}
	paid := &fakeBackend{name: "deepseek_api", paid: true}
	out, err := run(t, &Chain{Backends: []Backend{local, paid}}, "https://example.com")
	if err != nil || out != "status 200 OK · text/plain · 5 bytes\n\nhello" || paid.calls != 0 {
		t.Fatalf("out=%q err=%v paidCalls=%d", out, err, paid.calls)
	}
}

func TestChainFallsThroughOnEveryStructuralFailureWithItsKind(t *testing.T) {
	cases := map[string]*websearch.BackendError{
		"transport": {Kind: websearch.FailTransport, Err: errors.New("dial")},
		"status":    {Kind: websearch.FailStatus, Status: 403},
		"empty":     {Kind: websearch.FailEmpty},
	}
	for name, failure := range cases {
		t.Run(name, func(t *testing.T) {
			first := &fakeBackend{name: "a", err: failure}
			second := &fakeBackend{name: "b", page: Page{Rendered: "second", Text: "second"}}
			var falls []Fallback
			out, err := run(t, &Chain{Backends: []Backend{first, second}, OnFallback: func(f Fallback) { falls = append(falls, f) }}, "https://example.com")
			if err != nil || out != "second" || len(falls) != 1 || string(falls[0].Kind) != name || falls[0].Next != "b" {
				t.Fatalf("out=%q err=%v falls=%+v", out, err, falls)
			}
		})
	}
}

// A refusal to touch an internal address is a policy decision. It ends the
// chain: no other backend is asked to do what this machine would not.
func TestChainStopsAtAPolicyRefusal(t *testing.T) {
	local := &fakeBackend{name: "local", err: fmt.Errorf("%w 169.254.169.254", builtin.ErrFetchInternalAddress)}
	paid := &fakeBackend{name: "deepseek_api", paid: true, page: Page{Text: "x"}}
	_, err := run(t, &Chain{Backends: []Backend{local, paid}, Consent: func(context.Context, Backend, string) bool { return true }}, "http://169.254.169.254/")
	var ref tool.Refusal
	if !errors.As(err, &ref) || ref.Code != CodeInternalAddress || paid.calls != 0 {
		t.Fatalf("err=%v paidCalls=%d, want %s and no fall-through", err, paid.calls, CodeInternalAddress)
	}
}

// A URL naming this machine's own network is never offered to a paid third
// party, and the user is not asked to pay for it either.
func TestChainVetsTheURLBeforeAskingForConsentToAPaidBackend(t *testing.T) {
	local := &fakeBackend{name: "local", err: &websearch.BackendError{Kind: websearch.FailTransport}}
	paid := &fakeBackend{name: "deepseek_api", paid: true, page: Page{Text: "x"}}
	asked := false
	_, err := run(t, &Chain{
		Backends: []Backend{local, paid},
		Vet:      func(context.Context, string) error { return builtin.ErrFetchInternalAddress },
		Consent:  func(context.Context, Backend, string) bool { asked = true; return true },
	}, "http://10.0.0.5/admin")
	var ref tool.Refusal
	if !errors.As(err, &ref) || ref.Code != CodeInternalAddress || asked || paid.calls != 0 {
		t.Fatalf("err=%v asked=%v paidCalls=%d", err, asked, paid.calls)
	}
}

func TestChainNeverCallsAPaidBackendWithoutConsent(t *testing.T) {
	local := &fakeBackend{name: "local", err: &websearch.BackendError{Kind: websearch.FailStatus, Status: 403}}
	paid := &fakeBackend{name: "deepseek_api", paid: true, page: Page{Text: "x"}}
	_, err := run(t, &Chain{Backends: []Backend{local, paid}}, "https://example.com")
	var ref tool.Refusal
	if !errors.As(err, &ref) || ref.Code != CodeFreeFailedPaidNeedsConsent || paid.calls != 0 {
		t.Fatalf("err=%v paidCalls=%d", err, paid.calls)
	}
}

func TestChainMarksAndBillsAPaidPageOnlyWhenItAnswered(t *testing.T) {
	local := &fakeBackend{name: "local", err: &websearch.BackendError{Kind: websearch.FailStatus, Status: 403}}
	paid := &fakeBackend{name: "deepseek_api", paid: true, page: Page{Title: "T", Text: "body text"}}
	var billed []string
	var asked []string
	out, err := run(t, &Chain{Backends: []Backend{local, paid},
		Consent: func(_ context.Context, b Backend, url string) bool {
			asked = append(asked, b.Name()+" "+url)
			return true
		},
		OnBilled: func(name string) { billed = append(billed, name) }}, "https://example.com/a")
	if err != nil || len(billed) != 1 || billed[0] != "deepseek_api" || len(asked) != 1 || asked[0] != "deepseek_api https://example.com/a" {
		t.Fatalf("billed=%v asked=%v err=%v", billed, asked, err)
	}
	for _, want := range []string{"deepseek_api", "billed", "# T", "body text"} {
		if !strings.Contains(out, want) {
			t.Fatalf("paid page lacks %q: %s", want, out)
		}
	}
	empty := &fakeBackend{name: "deepseek_api", paid: true, err: &websearch.BackendError{Kind: websearch.FailEmpty}}
	billed = nil
	_, _ = run(t, &Chain{Backends: []Backend{empty}, Consent: func(context.Context, Backend, string) bool { return true }, OnBilled: func(n string) { billed = append(billed, n) }}, "https://example.com")
	if len(billed) != 0 {
		t.Fatalf("an empty paid answer was billed: %v", billed)
	}
}

func TestChainRefusalNamesEachFailureIdentity(t *testing.T) {
	local := &fakeBackend{name: "local", err: &websearch.BackendError{Backend: "local", Kind: websearch.FailStatus, Status: 403}}
	paid := &fakeBackend{name: "deepseek_api", paid: true, err: &websearch.BackendError{Backend: "deepseek_api", Kind: websearch.FailStatus, Status: 402, Code: websearch.CodeSearchInsufficientBalance}}
	_, err := run(t, &Chain{Backends: []Backend{local, paid}, Consent: func(context.Context, Backend, string) bool { return true }}, "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "local: status") || !strings.Contains(err.Error(), "deepseek_api: "+websearch.CodeSearchInsufficientBalance) || !strings.Contains(err.Error(), CodeAllBackendsFailed) {
		t.Fatalf("err = %v", err)
	}
}

func TestChainIsNotFooledByACancelledFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	first := &fakeBackend{name: "a", err: context.Canceled}
	second := &fakeBackend{name: "b", page: Page{Text: "x"}}
	_, err := (&Chain{Backends: []Backend{first, second}}).Execute(ctx, json.RawMessage(`{"url":"https://example.com"}`))
	if !errors.Is(err, context.Canceled) || second.calls != 0 {
		t.Fatalf("err=%v second=%d", err, second.calls)
	}
}

// The local backend judges its own answer by structure: the status the
// response carried and whether any text came back, never the text itself.
func TestLocalBackendJudgesByStatusAndEmptiness(t *testing.T) {
	cases := []struct {
		name string
		page builtin.FetchPage
		err  error
		kind websearch.FailKind
		ok   bool
	}{
		{"200 with text", builtin.FetchPage{Status: 200, StatusText: "200 OK", Text: "hi", Bytes: 2}, nil, "", true},
		{"204 empty", builtin.FetchPage{Status: 204}, nil, websearch.FailEmpty, false},
		{"403 with a page", builtin.FetchPage{Status: http.StatusForbidden, Text: "blocked"}, nil, websearch.FailStatus, false},
		{"transport", builtin.FetchPage{}, errors.New("dial"), websearch.FailTransport, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := NewLocal(stubFetcher{page: c.page, err: c.err})
			page, err := b.Fetch(context.Background(), "https://example.com")
			if c.ok {
				if err != nil || page.Text != "hi" || !strings.Contains(page.Rendered, "hi") {
					t.Fatalf("page=%+v err=%v", page, err)
				}
				return
			}
			var be *websearch.BackendError
			if !errors.As(err, &be) || be.Kind != c.kind {
				t.Fatalf("err = %v, want kind %s", err, c.kind)
			}
		})
	}
	policy := NewLocal(stubFetcher{err: fmt.Errorf("%w x", builtin.ErrFetchInternalAddress)})
	if _, err := policy.Fetch(context.Background(), "http://10.0.0.1"); !errors.Is(err, builtin.ErrFetchInternalAddress) {
		t.Fatalf("a policy refusal lost its identity: %v", err)
	}
}

type stubFetcher struct {
	page builtin.FetchPage
	err  error
}

func (s stubFetcher) Fetch(context.Context, string) (builtin.FetchPage, error) { return s.page, s.err }
