package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/ext/plugin"
)

func fakeOAuthMCP(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q`, srv.URL+"/.well-known/oauth-protected-resource"))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": srv.URL + "/mcp", "authorization_servers": []string{srv.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": srv.URL, "authorization_endpoint": srv.URL + "/authorize", "token_endpoint": srv.URL + "/token",
				"registration_endpoint": srv.URL + "/register", "code_challenge_methods_supported": []string{"S256"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
			})
		case "/register":
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "c", "client_secret": "s", "token_endpoint_auth_method": "client_secret_basic"})
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "refresh_token": "r", "token_type": "Bearer", "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func signInController(t *testing.T, serverURL string) (*Controller, chan event.Approval) {
	t.Helper()
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	body := "[[plugins]]\nname = \"figma\"\ntype = \"http\"\nurl = \"" + serverURL + "/mcp\"\n"
	if err := os.WriteFile(config.UserConfigPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := make(chan event.Approval, 4)
	root := testenv.TempDir(t)
	c := New(Options{WorkspaceRoot: root, MCPConfigureSpec: func(s *plugin.Spec) {
		s.StateDir = plugin.MCPStateDir(home, root, s.Name)
	}, Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			requests <- e.Approval
		}
	})})
	t.Cleanup(c.Close)
	return c, requests
}

func browserThatApproves(opened *atomic.Int32) func(string) error {
	return func(raw string) error {
		opened.Add(1)
		authURL, err := url.Parse(raw)
		if err != nil {
			return err
		}
		query := authURL.Query()
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
}

type signInAnswer struct {
	res MCPSignInResult
	err error
}

func signIn(c *Controller, open func(string) error) chan signInAnswer {
	done := make(chan signInAnswer, 1)
	go func() {
		res, err := c.authorizeMCPServer(context.Background(), "figma", open)
		done <- signInAnswer{res, err}
	}()
	return done
}

func settle(t *testing.T, done chan signInAnswer) signInAnswer {
	t.Helper()
	select {
	case got := <-done:
		return got
	case <-time.After(testenv.Budget(t)):
		t.Fatal("sign-in did not finish")
		return signInAnswer{}
	}
}

// The server picked the host, so a person is asked before any browser opens,
// whatever the approval mode; the card names the server and the host.
func TestMCPSignInAsksBeforeTheBrowserOpensEvenInYolo(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	c.SetAutoApproveTools(true)
	var opened atomic.Int32
	done := signIn(c, browserThatApproves(&opened))
	approval := nextApproval(t, requests)
	wantHost := plugin.AuthorizationHost(srv.URL)
	if approval.Tool != MCPSignInApprovalTool || approval.Subject != "figma → "+wantHost || !approval.Fresh || !approval.AllowsPersist {
		t.Fatalf("approval = %+v", approval)
	}
	if opened.Load() != 0 {
		t.Fatal("the browser opened before the user answered")
	}
	c.Approve(approval.ID, true, false, false)
	got := settle(t, done)
	if got.err != nil || got.res.Host != wantHost || opened.Load() != 1 {
		t.Fatalf("approved sign-in = %+v opened=%d", got, opened.Load())
	}
}

func TestMCPSignInDeclinedOpensNothingAndStoresNoToken(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	var opened atomic.Int32
	done := signIn(c, browserThatApproves(&opened))
	c.Approve(nextApproval(t, requests).ID, false, false, false)
	got := settle(t, done)
	if !errors.Is(got.err, ErrMCPSignInDeclined) || opened.Load() != 0 {
		t.Fatalf("declined = %+v opened=%d", got, opened.Load())
	}
	entry, _ := c.configuredMCPServer("figma")
	if _, err := os.Stat(filepath.Join(c.mcpSpec(entry).StateDir, "oauth.json")); err == nil {
		t.Fatal("a declined sign-in left a token behind")
	}
}

// "Do not ask again" belongs to one server and one host: the same server is not
// asked again, and removing the server's sign-in state takes the agreement with it.
func TestMCPSignInRememberedHostSkipsTheCardUntilTheStateIsCleared(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	var opened atomic.Int32
	done := signIn(c, browserThatApproves(&opened))
	c.Approve(nextApproval(t, requests).ID, true, false, true)
	if got := settle(t, done); got.err != nil {
		t.Fatal(got.err)
	}
	entry, _ := c.configuredMCPServer("figma")
	stateDir := c.mcpSpec(entry).StateDir
	if !plugin.SignInHostRemembered(stateDir, plugin.AuthorizationHost(srv.URL)) {
		t.Fatal("always allow did not record the host")
	}
	if got := settle(t, signIn(c, browserThatApproves(&opened))); got.err != nil || opened.Load() != 2 {
		t.Fatalf("second sign-in = %+v opened=%d", got, opened.Load())
	}
	select {
	case a := <-requests:
		t.Fatalf("a remembered host was asked again: %+v", a)
	default:
	}
	if _, err := plugin.ReconcileHTTPMCPOAuthAfterRemoval(plugin.Spec{Name: "figma", StateDir: stateDir}, ""); err != nil {
		t.Fatal(err)
	}
	if plugin.SignInHostRemembered(stateDir, plugin.AuthorizationHost(srv.URL)) {
		t.Fatal("clearing the sign-in state kept the agreement")
	}
}

func TestMCPSignInRefusalsNeedNoCard(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	if _, err := c.authorizeMCPServer(context.Background(), "nope", func(string) error { return nil }); err == nil {
		t.Fatal("an unconfigured server signed in")
	}
	if _, err := c.authorizeMCPServer(context.Background(), "figma", nil); !errors.Is(err, ErrMCPSignInNoBrowser) {
		t.Fatalf("no browser: %v", err)
	}
	if !c.mcp.beginSignIn("figma") {
		t.Fatal("first claim refused")
	}
	if _, err := c.authorizeMCPServer(context.Background(), "figma", func(string) error { return nil }); !errors.Is(err, ErrMCPSignInBusy) {
		t.Fatalf("second sign-in while one is open: %v", err)
	}
	c.mcp.endSignIn("figma")
	select {
	case a := <-requests:
		t.Fatalf("a refusal raised a card: %+v", a)
	default:
	}
}

func TestMCPSignInIneligibleServerIsRefused(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, _ := signInController(t, srv.URL)
	body := "[[plugins]]\nname = \"figma\"\ntype = \"http\"\nurl = \"" + srv.URL + "/mcp\"\n[plugins.headers]\nAuthorization = \"Bearer x\"\n"
	if err := os.WriteFile(config.UserConfigPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.authorizeMCPServer(context.Background(), "figma", func(string) error { return nil }); !errors.Is(err, ErrMCPSignInIneligible) {
		t.Fatalf("configured credentials: %v", err)
	}
}

func askForScopes(t *testing.T, c *Controller, scopes ...string) {
	t.Helper()
	entry, _ := c.configuredMCPServer("figma")
	dir := c.mcpSpec(entry).StateDir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"resource": "r", "scopes": scopes, "at": time.Now()})
	if err := os.WriteFile(filepath.Join(dir, "mcp-oauth-scope-challenge.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func upgrade(c *Controller, open func(string) error) chan signInAnswer {
	done := make(chan signInAnswer, 1)
	go func() {
		res, err := c.authorizeMCPServerWith(context.Background(), "figma", open, true)
		done <- signInAnswer{res, err}
	}()
	return done
}

// Widening access is the user's call every time: yolo cannot answer, the card
// carries the server's scope words, and it offers no way to remember the answer.
func TestMCPScopeUpgradeAsksEvenInYoloAndOffersNoRemember(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	c.SetAutoApproveTools(true)
	askForScopes(t, c, "files:write")
	var opened atomic.Int32
	done := upgrade(c, browserThatApproves(&opened))
	a := nextApproval(t, requests)
	if a.Tool != MCPScopeUpgradeApprovalTool || !a.Fresh || a.AllowsPersist || !strings.Contains(a.Reason, "files:write") {
		t.Fatalf("approval = %+v", a)
	}
	if opened.Load() != 0 {
		t.Fatal("the browser opened before the user answered")
	}
	c.Approve(a.ID, true, true, true)
	if got := settle(t, done); got.err != nil || opened.Load() != 1 {
		t.Fatalf("approved = %+v opened=%d", got, opened.Load())
	}
	entry, _ := c.configuredMCPServer("figma")
	if plugin.SignInHostRemembered(c.mcpSpec(entry).StateDir, plugin.AuthorizationHost(srv.URL)) {
		t.Fatal("an upgrade answer was remembered as a sign-in host")
	}
	askForScopes(t, c, "files:write")
	again := upgrade(c, browserThatApproves(&opened))
	c.Approve(nextApproval(t, requests).ID, true, false, false)
	if got := settle(t, again); got.err != nil {
		t.Fatalf("a session grant answered the next upgrade: %+v", got)
	}
}

func TestMCPScopeUpgradeStillAsksAfterAHostWasRemembered(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	var opened atomic.Int32
	done := signIn(c, browserThatApproves(&opened))
	c.Approve(nextApproval(t, requests).ID, true, false, true)
	if got := settle(t, done); got.err != nil {
		t.Fatal(got.err)
	}
	askForScopes(t, c, "files:write")
	up := upgrade(c, browserThatApproves(&opened))
	c.Approve(nextApproval(t, requests).ID, true, false, false)
	if got := settle(t, up); got.err != nil {
		t.Fatal(got.err)
	}
}

func TestMCPScopeUpgradeDeclinedOpensNothing(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	askForScopes(t, c, "files:write")
	var opened atomic.Int32
	done := upgrade(c, browserThatApproves(&opened))
	c.Approve(nextApproval(t, requests).ID, false, false, false)
	if got := settle(t, done); !errors.Is(got.err, ErrMCPSignInDeclined) || opened.Load() != 0 {
		t.Fatalf("declined = %+v opened=%d", got, opened.Load())
	}
}

func TestMCPScopeUpgradeWithNothingAskedRaisesNoCardAndNoBrowser(t *testing.T) {
	srv := fakeOAuthMCP(t)
	c, requests := signInController(t, srv.URL)
	var opened atomic.Int32
	got := settle(t, upgrade(c, browserThatApproves(&opened)))
	if !errors.Is(got.err, ErrMCPNoScopeUpgrade) || opened.Load() != 0 {
		t.Fatalf("got %+v opened=%d", got, opened.Load())
	}
	select {
	case a := <-requests:
		t.Fatalf("raised a card: %+v", a)
	default:
	}
}
