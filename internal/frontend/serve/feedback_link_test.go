package serve

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/platform/feedback"
	"reasonix/internal/session/control"
)

type linkUpstream struct {
	mu   sync.Mutex
	seen []*http.Request
	code int
	body string
}

func (u *linkUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.seen = append(u.seen, r.Clone(context.Background()))
	code, body := u.code, u.body
	u.mu.Unlock()
	if strings.HasSuffix(r.URL.Path, "/v1/feedback") && r.Method == http.MethodPost {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"receipt":"FB-7K3M-9QX2","status":"received","installToken":"tok-secret","createdAt":"2026-09-30T08:00:00Z"}`)
		return
	}
	if code != 0 {
		w.WriteHeader(code)
	}
	_, _ = io.WriteString(w, body)
}

func (u *linkUpstream) last() *http.Request {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.seen[len(u.seen)-1]
}

func linkServer(t *testing.T, up *linkUpstream, signedIn bool) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(up)
	t.Cleanup(upstream.Close)
	cfg := feedback.Config{Home: testenv.TempDir(t), Base: upstream.URL, HTTP: upstream.Client(), Backoff: []time.Duration{}}
	if signedIn {
		cfg.Assert = func(context.Context) (string, error) { return "v1.proof.sig", nil }
	}
	svc, err := feedback.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	opts := control.Options{Runner: fakeRunner{}, WorkspaceRoot: testenv.TempDir(t), Feedback: control.FeedbackOptions{Service: svc, Surface: feedback.SurfaceStudio, ProviderKind: "deepseek"}}
	bc := NewBroadcaster()
	opts.Sink = bc
	srv := httptest.NewServer(operatorHandler(New(control.New(opts), bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)
	return srv
}

func sendReport(t *testing.T, base string) {
	t.Helper()
	resp, out := feedbackPost(t, base+"/feedback", map[string]any{"idempotencyKey": "k-1", "category": "bug", "body": "the first report body", "displayName": "kim", "locale": "en-US"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit %d: %s", resp.StatusCode, out)
	}
}

func do(t *testing.T, method, url string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

func TestLinkingIsRefusedBeforeAnyReportExistsAndWhileSignedOut(t *testing.T) {
	up := &linkUpstream{}
	srv := linkServer(t, up, true)
	resp, out := feedbackPost(t, srv.URL+"/feedback/link", nil)
	if resp.StatusCode != http.StatusConflict || reasonOf(t, out).Code != codeFeedbackNoInstall {
		t.Fatalf("no install: %d %s", resp.StatusCode, out)
	}
	signedOut := linkServer(t, &linkUpstream{}, false)
	sendReport(t, signedOut.URL)
	resp, out = feedbackPost(t, signedOut.URL+"/feedback/link", nil)
	if resp.StatusCode != http.StatusUnauthorized || reasonOf(t, out).Code != codeFeedbackAccount {
		t.Fatalf("signed out: %d %s", resp.StatusCode, out)
	}
	resp, out = do(t, http.MethodGet, signedOut.URL+"/feedback/links")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed-out list: %d %s", resp.StatusCode, out)
	}
	if len(up.seen) != 0 {
		t.Fatal("a refused link reached the service")
	}
}

func TestLinkAndUnlinkReachTheServiceWithTheProofAndNeverReturnTheInstallToken(t *testing.T) {
	up := &linkUpstream{body: `{"linked":true}`}
	srv := linkServer(t, up, true)
	sendReport(t, srv.URL)
	resp, out := feedbackPost(t, srv.URL+"/feedback/link", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("link %d: %s", resp.StatusCode, out)
	}
	r := up.last()
	if r.Method != http.MethodPost || r.URL.Path != "/v1/feedback/link" || r.Header.Get("X-Account-Assertion") != "v1.proof.sig" || r.Header.Get("X-Install-Token") != "tok-secret" {
		t.Fatalf("upstream saw %s %s %v", r.Method, r.URL.Path, r.Header)
	}
	if resp, out = do(t, http.MethodDelete, srv.URL+"/feedback/link"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unlink %d: %s", resp.StatusCode, out)
	}
	if up.last().Method != http.MethodDelete {
		t.Fatalf("unlink sent %s", up.last().Method)
	}
	up.mu.Lock()
	up.body = `{"installs":[{"id":"0123456789ab","linkedAt":"2026-10-15T00:00:00Z","current":true}]}`
	up.mu.Unlock()
	resp, out = do(t, http.MethodGet, srv.URL+"/feedback/links")
	if resp.StatusCode != http.StatusOK || strings.Contains(string(out), "tok-secret") || !strings.Contains(string(out), `"id":"0123456789ab"`) {
		t.Fatalf("list %d: %s", resp.StatusCode, out)
	}
	resp, out = do(t, http.MethodDelete, srv.URL+"/feedback/links/0123456789ab")
	if resp.StatusCode != http.StatusNoContent || up.last().URL.Path != "/v1/feedback/link/0123456789ab" {
		t.Fatalf("unlink one %d: %s %s", resp.StatusCode, out, up.last().URL.Path)
	}
}

func TestLinkConflictAndLimitKeepTheirOwnCodes(t *testing.T) {
	for _, c := range []struct{ code, want string }{
		{"feedback.link_conflict", codeFeedbackLinkConflict},
		{"feedback.link_limit", codeFeedbackLinkLimit},
	} {
		up := &linkUpstream{code: 403, body: `{"error":{"code":"` + c.code + `","message":"x"}}`}
		srv := linkServer(t, up, true)
		sendReport(t, srv.URL)
		resp, out := feedbackPost(t, srv.URL+"/feedback/link", nil)
		if resp.StatusCode != http.StatusConflict || reasonOf(t, out).Code != c.want {
			t.Fatalf("%s: %d %s", c.code, resp.StatusCode, out)
		}
	}
}
