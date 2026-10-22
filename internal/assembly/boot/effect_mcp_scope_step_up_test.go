package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/session/control"
)

// stepUpServer is an OAuth-protected MCP server that issues least-privilege
// tokens: the first sign-in yields files:read, and a call to "write" is refused
// with a scope challenge until a token carrying files:write has been issued.
type stepUpServer struct {
	*httptest.Server
	mu         sync.Mutex
	issued     int
	calls      []string
	stillDenys bool
}

func newStepUpServer(t *testing.T, stillDenys bool) *stepUpServer {
	t.Helper()
	s := &stepUpServer{stillDenys: stillDenys}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			s.mcp(w, r)
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": s.URL + "/mcp", "authorization_servers": []string{s.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": s.URL, "authorization_endpoint": s.URL + "/authorize", "token_endpoint": s.URL + "/token",
				"registration_endpoint": s.URL + "/register", "code_challenge_methods_supported": []string{"S256"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
			})
		case "/register":
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "c", "client_secret": "s", "token_endpoint_auth_method": "client_secret_basic"})
		case "/token":
			s.mu.Lock()
			s.issued++
			n := s.issued
			s.mu.Unlock()
			scope := "files:read"
			if n > 1 {
				scope = "files:read files:write"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("token-%d", n), "refresh_token": "r", "token_type": "Bearer", "expires_in": 3600, "scope": scope})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *stepUpServer) mcp(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q`, s.URL+"/.well-known/oauth-protected-resource"))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		ID     *int   `json:"id"`
		Method string `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var result any
	switch req.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "figma", "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}}
	case "tools/list":
		result = map[string]any{"tools": []map[string]any{{"name": "write", "description": "Write.", "inputSchema": map[string]any{"type": "object"}}}}
	case "tools/call":
		s.mu.Lock()
		s.calls = append(s.calls, token)
		s.mu.Unlock()
		if token == "token-1" || s.stillDenys {
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="files:write"`)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		result = map[string]any{"content": []map[string]any{{"type": "text", "text": "written with " + token}}}
	default:
		http.Error(w, "unsupported", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
}

func (s *stepUpServer) callTokens() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// stepUpBrowser stands in for the person's browser: it follows the redirect the
// authorization page would send back, and records which scopes it was asked for.
type stepUpBrowser struct {
	opened atomic.Int32
	mu     sync.Mutex
	scopes []string
}

func (b *stepUpBrowser) open(raw string) error {
	b.opened.Add(1)
	authURL, err := url.Parse(raw)
	if err != nil {
		return err
	}
	query := authURL.Query()
	b.mu.Lock()
	b.scopes = append(b.scopes, query.Get("scope"))
	b.mu.Unlock()
	callback, err := url.Parse(query.Get("redirect_uri"))
	if err != nil {
		return err
	}
	values := callback.Query()
	values.Set("code", "code")
	values.Set("state", query.Get("state"))
	callback.RawQuery = values.Encode()
	go func() {
		if resp, err := http.Get(callback.String()); err == nil {
			_ = resp.Body.Close()
		}
	}()
	return nil
}

// A step-up is the one place a server can ask for more than it was given, so the
// whole path is held at the boundary a person and a model actually meet: the
// model reads a typed cause and the user is told, with no browser opened until
// they act and agree to a card the server cannot answer.
func TestEffectMCPScopeChallengeReachesTheModelAndTheUserBeforeAnyBrowser(t *testing.T) {
	rec := useStepUpProvider()
	server := newStepUpServer(t, false)
	ctrl, approvals, notices := stepUpController(t, server.URL)
	browser := &stepUpBrowser{}

	signIn := func(upgrade bool) error {
		done := make(chan error, 1)
		go func() {
			var err error
			if upgrade {
				_, err = ctrl.UpgradeMCPServerAccess(context.Background(), "figma", browser.open)
			} else {
				_, err = ctrl.AuthorizeMCPServer(context.Background(), "figma", browser.open)
			}
			done <- err
		}()
		card := nextEffectApproval(t, approvals)
		ctrl.Approve(card.ID, true, false, false)
		select {
		case err := <-done:
			return err
		case <-time.After(testenv.Budget(t)):
			t.Fatal("sign-in did not finish")
			return nil
		}
	}

	if err := signIn(false); err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	if err := ctrl.Run(context.Background(), "write"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	results := effectToolResults(rec.last())
	if len(results) != 1 || !strings.Contains(results[0], "mcp.insufficient_scope") || !strings.Contains(results[0], "Only the user can authorize more") {
		t.Fatalf("the model read %q, want the typed scope refusal", results)
	}
	if strings.Contains(results[0], "files:write") {
		t.Fatalf("the server's scope text reached the model: %q", results[0])
	}
	if !hasNotice(notices(), event.NoticeCodeMCPScopeUpgrade) {
		t.Fatal("the user was not told the server wants more access")
	}
	if browser.opened.Load() != 1 {
		t.Fatalf("a browser opened without the user acting: %d opens, want only the sign-in", browser.opened.Load())
	}

	if err := signIn(true); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if browser.opened.Load() != 2 || !strings.Contains(browser.scopes[1], "files:write") || !strings.Contains(browser.scopes[1], "files:read") {
		t.Fatalf("the upgrade asked for %v, want the old scope and the new one together", browser.scopes)
	}
	if err := ctrl.Run(context.Background(), "write again"); err != nil {
		t.Fatalf("Run after upgrade: %v", err)
	}
	if got := effectToolResults(rec.last()); len(got) != 2 || !strings.Contains(got[1], "written with token-2") {
		t.Fatalf("after the upgrade the model read %q", got)
	}
	if calls := server.callTokens(); len(calls) != 2 || calls[0] != "token-1" || calls[1] != "token-2" {
		t.Fatalf("server saw calls with %v: the first refused, the retry on the new token", calls)
	}
}

// Declining the card ends it: no browser, no new token, and nothing is retried
// behind the person's back.
func TestEffectMCPScopeUpgradeDeclinedSendsNothingMore(t *testing.T) {
	useStepUpProvider()
	server := newStepUpServer(t, false)
	ctrl, approvals, _ := stepUpController(t, server.URL)
	browser := &stepUpBrowser{}

	done := make(chan error, 1)
	go func() { _, err := ctrl.AuthorizeMCPServer(context.Background(), "figma", browser.open); done <- err }()
	ctrl.Approve(nextEffectApproval(t, approvals).ID, true, false, false)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Run(context.Background(), "write"); err != nil {
		t.Fatal(err)
	}
	before := len(server.callTokens())

	go func() {
		_, err := ctrl.UpgradeMCPServerAccess(context.Background(), "figma", browser.open)
		done <- err
	}()
	ctrl.Approve(nextEffectApproval(t, approvals).ID, false, false, false)
	if err := <-done; err == nil {
		t.Fatal("a declined upgrade reported success")
	}
	if browser.opened.Load() != 1 || len(server.callTokens()) != before {
		t.Fatalf("declined: %d browser opens, %d calls (was %d): something moved without the person's yes", browser.opened.Load(), len(server.callTokens()), before)
	}
	server.mu.Lock()
	issued := server.issued
	server.mu.Unlock()
	if issued != 1 {
		t.Fatalf("a declined upgrade still produced a token: %d issued", issued)
	}
}

// A server that keeps refusing after the user agreed is final for that call: the
// model is told to stop, not to retry.
func TestEffectMCPScopeUpgradeThatChangesNothingIsFinal(t *testing.T) {
	rec := useStepUpProvider()
	server := newStepUpServer(t, true)
	ctrl, approvals, _ := stepUpController(t, server.URL)
	browser := &stepUpBrowser{}

	done := make(chan error, 1)
	go func() { _, err := ctrl.AuthorizeMCPServer(context.Background(), "figma", browser.open); done <- err }()
	ctrl.Approve(nextEffectApproval(t, approvals).ID, true, false, false)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Run(context.Background(), "write"); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, err := ctrl.UpgradeMCPServerAccess(context.Background(), "figma", browser.open)
		done <- err
	}()
	ctrl.Approve(nextEffectApproval(t, approvals).ID, true, false, false)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := ctrl.Run(context.Background(), "write once more"); err != nil {
		t.Fatal(err)
	}
	results := effectToolResults(rec.last())
	if len(results) != 2 || !strings.Contains(results[1], "mcp.insufficient_scope_permanent") || !strings.Contains(results[1], "Do not retry") {
		t.Fatalf("after an upgrade that changed nothing the model read %q", results)
	}
}

var (
	stepUpOnce sync.Once
	stepUpMu   sync.Mutex
	stepUpRec  *capabilityCallProvider
)

// useStepUpProvider gives the test a fresh scripted model that calls the
// server's write tool on each prompt; the kind is registered once per process.
func useStepUpProvider() *capabilityCallProvider {
	stepUpOnce.Do(func() {
		provider.Register("boot-mcp-step-up", func(provider.Config) (provider.Provider, error) {
			stepUpMu.Lock()
			defer stepUpMu.Unlock()
			return stepUpRec, nil
		})
	})
	stepUpMu.Lock()
	defer stepUpMu.Unlock()
	stepUpRec = &capabilityCallProvider{direct: "mcp__figma__write"}
	return stepUpRec
}

func stepUpController(t *testing.T, serverURL string) (*control.Controller, chan event.Approval, func() []event.Event) {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[codegraph]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-mcp-step-up"
model = "x"
`)
	user := config.UserConfigPath()
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, fmt.Appendf(nil, "[[plugins]]\nname = \"figma\"\ntype = \"http\"\nurl = %q\n", serverURL+"/mcp"), 0o600); err != nil {
		t.Fatal(err)
	}
	approveWorkspace(t, dir)
	approvals := make(chan event.Approval, 8)
	var mu sync.Mutex
	var seen []event.Event
	ctrl, err := Build(context.Background(), Options{Sink: event.FuncSink(func(e event.Event) {
		mu.Lock()
		seen = append(seen, e)
		mu.Unlock()
		if e.Kind == event.ApprovalRequest {
			approvals <- e.Approval
		}
	})})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(ctrl.Close)
	return ctrl, approvals, func() []event.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]event.Event(nil), seen...)
	}
}

func nextEffectApproval(t *testing.T, approvals chan event.Approval) event.Approval {
	t.Helper()
	select {
	case a := <-approvals:
		return a
	case <-time.After(testenv.Budget(t)):
		t.Fatal("no approval card was raised")
		return event.Approval{}
	}
}

func hasNotice(events []event.Event, code string) bool {
	for _, e := range events {
		if e.Kind == event.Notice && e.Code == code {
			return true
		}
	}
	return false
}
