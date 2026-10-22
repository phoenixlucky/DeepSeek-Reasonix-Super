package control

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/i18n"
	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/platform/feedback"
)

const linkRigAccount = `{"installs":[{"id":"aaaaaaaaaaaa","linkedAt":"2026-10-02T00:00:00Z","current":true},{"id":"bbbbbbbbbbbb","linkedAt":"2026-10-03T00:00:00Z","current":false}],
"items":[{"receipt":"FB-BBBB-0002","category":"idea","titleSnippet":"from the laptop","status":"received","createdAt":"2026-10-02T08:00:00Z","updatedAt":"2026-10-02T09:00:00Z","replies":[],"linkedHere":false}]}`

type linkRig struct {
	c      *Controller
	mu     sync.Mutex
	texts  []string
	calls  []string
	refuse map[string]string
}

func newLinkRig(t *testing.T, assertion string) *linkRig {
	t.Helper()
	r := &linkRig{refuse: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.calls = append(r.calls, req.Method+" "+req.URL.Path+" "+req.Header.Get("X-Account-Assertion"))
		code := r.refuse[req.Method+" "+req.URL.Path]
		r.mu.Unlock()
		if code != "" {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"`+code+`"}}`)
			return
		}
		switch {
		case req.URL.Path == "/v1/feedback" && req.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"receipt":"FB-AAAA-0001","status":"received","installToken":"t","createdAt":"2026-09-30T08:00:00Z"}`)
		case req.URL.Path == "/v1/feedback/mine":
			_, _ = io.WriteString(w, `{"items":[]}`)
		case req.URL.Path == "/v1/feedback/account/mine":
			_, _ = io.WriteString(w, linkRigAccount)
		case req.URL.Path == "/v1/feedback/link" && req.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"installs":[{"id":"aaaaaaaaaaaa","linkedAt":"2026-10-02T00:00:00Z","current":true},{"id":"bbbbbbbbbbbb","linkedAt":"2026-10-03T00:00:00Z","current":false}]}`)
		case req.URL.Path == "/v1/feedback/board" && req.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"view":"release","release":"v2.1.0","entries":[{"displayName":"Sample User","credits":2,"rank":1,"releases":[{"version":"v2.1.0","count":2}]},{"displayName":"Other Sample","credits":1,"rank":2,"releases":[{"version":"v2.1.0","count":1}]}]}`)
		case req.URL.Path == "/v1/feedback/board/me":
			_, _ = io.WriteString(w, `{"listed":true,"displayName":"Sample User","delisted":false}`)
		default:
			_, _ = io.WriteString(w, "{}")
		}
	}))
	t.Cleanup(srv.Close)
	cfg := feedback.Config{Home: testenv.TempDir(t), Base: srv.URL, HTTP: srv.Client(), Backoff: []time.Duration{}}
	if assertion != "" {
		cfg.Assert = func(context.Context) (string, error) { return assertion, nil }
	}
	svc, err := feedback.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r.c = New(Options{Feedback: FeedbackOptions{Surface: feedback.SurfaceTUI, Service: svc}, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice {
			r.mu.Lock()
			r.texts = append(r.texts, e.Text)
			r.mu.Unlock()
		}
	})})
	r.c.Submit("/feedback name kim")
	r.say(t, "nickname set")
	r.c.Submit("/feedback bug --yes x")
	r.say(t, "Receipt")
	return r
}

func (r *linkRig) say(t *testing.T, contains string) string {
	t.Helper()
	deadline := time.Now().Add(testenv.Budget(t))
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, s := range r.texts {
			if strings.Contains(s, contains) {
				r.mu.Unlock()
				return s
			}
		}
		r.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no notice containing %q; got %q", contains, r.texts)
	return ""
}

func (r *linkRig) called(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if strings.Contains(c, path) {
			n++
		}
	}
	return n
}

func TestFeedbackLinkSaysWhatItDoesAndNeedsAnExplicitYes(t *testing.T) {
	r := newLinkRig(t, "assert-1")
	r.c.Submit("/feedback link")
	r.say(t, "Nothing was sent")
	if r.called("POST /v1/feedback/link") != 0 {
		t.Fatal("the install was linked without --yes")
	}
	r.c.Submit("/feedback link --yes")
	got := r.say(t, "Linked")
	if r.called("POST /v1/feedback/link assert-1") != 1 {
		t.Fatalf("the link did not carry the assertion:\n%v", r.calls)
	}
	if !strings.Contains(got, "/feedback board join <name>") {
		t.Fatalf("linking did not say how to join the board:\n%s", got)
	}
	if n := r.called("PUT /v1/feedback/board/optin") + r.called("POST /v1/feedback/board/optin"); n != 0 {
		t.Fatalf("linking joined the board on its own:\n%v", r.calls)
	}
}

func TestFeedbackLinkSignedOutTellsThePersonToSignIn(t *testing.T) {
	r := newLinkRig(t, "")
	r.c.Submit("/feedback link --yes")
	r.say(t, "reasonix login")
	if r.called("/v1/feedback/link") != 0 {
		t.Fatal("a request went out with no sign-in")
	}
}

func TestFeedbackLinkRefusalsAreSaidFromTheirIdentity(t *testing.T) {
	r := newLinkRig(t, "assert-1")
	for code, says := range map[string]string{"feedback.link_conflict": "another account", "feedback.link_limit": "limit"} {
		r.mu.Lock()
		r.refuse["POST /v1/feedback/link"] = code
		r.mu.Unlock()
		r.c.Submit("/feedback link --yes")
		r.say(t, says)
	}
}

func TestFeedbackUnlinkAndLinksUseTheSameCalls(t *testing.T) {
	r := newLinkRig(t, "assert-1")
	r.c.Submit("/feedback links")
	got := r.say(t, "aaaaaaaaaaaa")
	if !strings.Contains(got, "this device") || !strings.Contains(got, "bbbbbbbbbbbb") {
		t.Fatalf("links = %q", got)
	}
	r.c.Submit("/feedback unlink")
	r.say(t, "Nothing was sent")
	if r.called("DELETE /v1/feedback/link") != 0 {
		t.Fatal("unlinked without --yes")
	}
	r.c.Submit("/feedback unlink --yes")
	r.say(t, "Unlinked")
	if r.called("DELETE /v1/feedback/link assert-1") != 1 {
		t.Fatalf("calls: %v", r.calls)
	}
}

func TestFeedbackListMarksAReportFromAnotherDevice(t *testing.T) {
	r := newLinkRig(t, "assert-1")
	r.c.Submit("/feedback link --yes")
	r.say(t, "Linked")
	r.c.Submit("/feedback list")
	got := r.say(t, "from the laptop")
	if !strings.Contains(got, "[other device]") {
		t.Fatalf("list = %q", got)
	}
}

func TestFeedbackLinkWordsAreInEveryCatalogue(t *testing.T) {
	for tag, m := range map[string]i18n.Messages{"en": i18n.English, "zh": i18n.Chinese, "zh-TW": i18n.ChineseTraditional} {
		if m.Feedback.LinkExplain == "" || m.Feedback.LinkDone == "" || m.Feedback.LinkBoardHint == "" || m.Feedback.NeedSignIn == "" || m.Feedback.OtherDeviceMark == "" {
			t.Errorf("%s: link words are missing", tag)
		}
	}
}

func TestFeedbackBoardListsWithPlacesForAReleaseAndNeedsNoAccountToRead(t *testing.T) {
	r := newLinkRig(t, "")
	r.c.Submit("/feedback board v2.1.0")
	got := r.say(t, "Sample User")
	if !strings.Contains(got, "1. Sample User - 2 adopted") || !strings.Contains(got, "2. Other Sample - 1 adopted") {
		t.Fatalf("board = %q", got)
	}
	if r.called("/v1/feedback/board v") != 0 || strings.Contains(strings.Join(r.calls, "\n"), "GET /v1/feedback/board assert") {
		t.Fatalf("a public read carried an assertion: %v", r.calls)
	}
}

func TestFeedbackBoardJoinSaysTheNameFirstAndSendsNothingUntilYes(t *testing.T) {
	r := newLinkRig(t, "assert-1")
	r.c.Submit("/feedback board join Sample Person")
	r.say(t, "\"Sample Person\"")
	if r.called("PUT /v1/feedback/board/optin") != 0 {
		t.Fatal("the account was put on the board without --yes")
	}
	r.c.Submit("/feedback board join Sample Person --yes")
	r.say(t, "You are on the board")
	if r.called("PUT /v1/feedback/board/optin assert-1") != 1 {
		t.Fatalf("the join did not carry the assertion:\n%v", r.calls)
	}
	r.c.Submit("/feedback board leave")
	r.say(t, "You are off the board")
	if r.called("DELETE /v1/feedback/board/optin assert-1") != 1 {
		t.Fatalf("the leave did not carry the assertion:\n%v", r.calls)
	}
}

func TestFeedbackBoardJoinSignedOutAndRefusedAreSaidFromTheirIdentity(t *testing.T) {
	out := newLinkRig(t, "")
	out.c.Submit("/feedback board join Sample --yes")
	out.say(t, "reasonix login")
	if out.called("/v1/feedback/board/optin") != 0 {
		t.Fatal("a request went out with no sign-in")
	}
	r := newLinkRig(t, "assert-1")
	for code, says := range map[string]string{"feedback.board_name_reserved": "reserved", "feedback.board_name_taken": "already uses", "feedback.board_delisted": "removed", "feedback.board_no_link": "/feedback link"} {
		r.mu.Lock()
		r.refuse["PUT /v1/feedback/board/optin"] = code
		r.mu.Unlock()
		r.c.Submit("/feedback board join Sample --yes")
		r.say(t, says)
	}
}
