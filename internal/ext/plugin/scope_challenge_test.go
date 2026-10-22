package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

// MCP 2025-11-25, Authorization, "Runtime Insufficient Scope Errors" (read
// 2026-10-18): the example 403 answer, verbatim.
const specInsufficientScope = `Bearer error="insufficient_scope",
                         scope="files:read files:write user:profile",
                         resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource",
                         error_description="Additional file write permission required"`

func TestTheSpecsInsufficientScopeChallengeIsRead(t *testing.T) {
	c, ok := parseBearerChallengeFull(strings.Join(strings.Fields(specInsufficientScope), " "))
	if !ok || c.Error != "insufficient_scope" || c.Scope != "files:read files:write user:profile" ||
		c.MetadataURL != "https://mcp.example.com/.well-known/oauth-protected-resource" {
		t.Fatalf("challenge = %+v ok=%v", c, ok)
	}
}

func TestAChallengeScopeIsAListOfTokensAndNothingElse(t *testing.T) {
	long := strings.Repeat("x", maxScopeLength+1)
	var many []string
	for i := range maxChallengeScopes + 10 {
		many = append(many, "s"+strings.Repeat("a", i%5)+string(rune('A'+i%26))+string(rune('a'+i/26)))
	}
	for raw, want := range map[string][]string{
		`files:read  files:write`:     {"files:read", "files:write"},
		`a a a b`:                     {"a", "b"},
		"a\tb\nc":                     {"a", "b", "c"},
		`ok "quoted" back\slash fine`: {"ok", "fine"},
		long + " kept":                {"kept"},
		"snow☃ café plain":            {"plain"},
		"":                            nil,
	} {
		if got := scopeTokens(raw); !slices.Equal(got, want) {
			t.Errorf("scopeTokens(%q) = %v, want %v", raw, got, want)
		}
	}
	if got := scopeTokens(strings.Join(many, " ")); len(got) != maxChallengeScopes {
		t.Fatalf("%d scopes kept, want the cap %d", len(got), maxChallengeScopes)
	}
}

// signedIn is a transport that holds an OAuth session, pointed at a server that
// answers every call with the given status and challenge.
func signedIn(t *testing.T, status int, challenge string) (*httpTransport, string) {
	t.Helper()
	stateDir := testenv.TempDir(t)
	if err := saveMCPOAuthState(stateDir, mcpOAuthState{Version: 1, AccessToken: "tok", Scope: "files:read"}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if challenge != "" {
			w.Header().Set("WWW-Authenticate", challenge)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	oauth, err := newMCPOAuthClient(stateDir, nil)
	if err != nil || oauth == nil {
		t.Fatalf("oauth client = %v, %v", oauth, err)
	}
	return &httpTransport{name: "files", url: srv.URL + "/mcp", client: srv.Client(), oauth: oauth}, stateDir
}

func failureOf(t *testing.T, tr *httpTransport, op string) error {
	t.Helper()
	resp, err := http.Get(tr.url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return tr.statusFailure(resp, 0, nil, op)
}

func TestA403InsufficientScopeIsTypedAndItsScopesAreKept(t *testing.T) {
	tr, stateDir := signedIn(t, http.StatusForbidden, `Bearer error="insufficient_scope", scope="files:read files:write"`)
	err := failureOf(t, tr, "tools/call:write_file")
	var need *InsufficientScopeError
	if !errors.As(err, &need) || !errors.Is(err, ErrInsufficientScope) || errors.Is(err, ErrScopeUpgradeExhausted) {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(need.Scopes, []string{"files:read", "files:write"}) || need.Server != "files" {
		t.Fatalf("error = %+v", need)
	}
	var status *httpStatusError
	if !errors.As(err, &status) || status.Status != http.StatusForbidden {
		t.Fatal("the HTTP status is no longer readable through the typed error")
	}
	if got := PendingScopeUpgrade(stateDir); !slices.Equal(got, []string{"files:read", "files:write"}) {
		t.Fatalf("pending = %v", got)
	}
}

func TestAPlain403IsNotAStepUpChallenge(t *testing.T) {
	for name, challenge := range map[string]string{
		"no challenge":          "",
		"another error":         `Bearer error="invalid_token"`,
		"a basic challenge":     `Basic realm="x"`,
		"a scope with no error": `Bearer scope="files:write"`,
	} {
		tr, stateDir := signedIn(t, http.StatusForbidden, challenge)
		if err := failureOf(t, tr, "tools/call:x"); errors.Is(err, ErrInsufficientScope) || PendingScopeUpgrade(stateDir) != nil {
			t.Errorf("%s: read as a scope challenge: %v", name, err)
		}
	}
}

// With a key or header the user configured, a 403 is that credential's answer:
// there is no sign-in to extend.
func TestAStaticCredentialIsNeverSteppedUp(t *testing.T) {
	tr, stateDir := signedIn(t, http.StatusForbidden, `Bearer error="insufficient_scope", scope="files:write"`)
	tr.headers = map[string]string{"Authorization": "Bearer configured"}
	if err := failureOf(t, tr, "tools/call:x"); errors.Is(err, ErrInsufficientScope) || PendingScopeUpgrade(stateDir) != nil {
		t.Fatalf("a configured credential was offered a step-up: %v", err)
	}
}

func TestWithoutASignInThereIsNothingToExtend(t *testing.T) {
	tr, _ := signedIn(t, http.StatusForbidden, `Bearer error="insufficient_scope", scope="files:write"`)
	tr.oauth = nil
	if err := failureOf(t, tr, "tools/call:x"); errors.Is(err, ErrInsufficientScope) {
		t.Fatalf("an unsigned server was offered a step-up: %v", err)
	}
}

// Retrying the original request is limited: a server that names only scopes that
// were already granted, or an operation that keeps coming back, is a permanent
// failure, tracked per operation.
func TestAnOperationThatKeepsBeingRefusedIsAPermanentFailure(t *testing.T) {
	tr, stateDir := signedIn(t, http.StatusForbidden, `Bearer error="insufficient_scope", scope="files:write"`)
	for i := range maxScopeUpgrades {
		if err := failureOf(t, tr, "tools/call:write_file"); errors.Is(err, ErrScopeUpgradeExhausted) {
			t.Fatalf("challenge %d was already permanent", i+1)
		}
	}
	if err := failureOf(t, tr, "tools/call:write_file"); !errors.Is(err, ErrScopeUpgradeExhausted) {
		t.Fatalf("the operation was never stopped: %v", err)
	}
	if err := failureOf(t, tr, "tools/call:delete_file"); errors.Is(err, ErrScopeUpgradeExhausted) {
		t.Fatal("one operation's refusals stopped another")
	}
	if err := saveMCPOAuthState(stateDir, mcpOAuthState{Version: 1, AccessToken: "tok", Scope: "files:read files:write"}); err != nil {
		t.Fatal(err)
	}
	if err := failureOf(t, tr, "tools/call:other"); !errors.Is(err, ErrScopeUpgradeExhausted) {
		t.Fatalf("a challenge for a scope that was already granted is not permanent: %v", err)
	}
}

func TestAnUpgradeKeepsWhatWasAlreadyGranted(t *testing.T) {
	if got := mergeScopes("files:read mcp:connect", []string{"files:write", "files:read"}); got != "files:read mcp:connect files:write" {
		t.Fatalf("scope = %q", got)
	}
	if got := mergeScopes("", []string{"a"}); got != "a" {
		t.Fatalf("scope = %q", got)
	}
}

// authServer is the smallest server and authorization server the flow needs; it
// records the scope the authorization request asked for.
func authServer(t *testing.T, asked *string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+srv.URL+`/.well-known/oauth-protected-resource"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource":
			write(map[string]any{"resource": srv.URL + "/mcp", "authorization_servers": []string{srv.URL}, "scopes_supported": []string{"files:read"}})
		case "/.well-known/oauth-authorization-server":
			write(map[string]any{"issuer": srv.URL, "authorization_endpoint": srv.URL + "/authorize", "token_endpoint": srv.URL + "/token",
				"registration_endpoint": srv.URL + "/register", "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}})
		case "/register":
			write(map[string]any{"client_id": "c", "token_endpoint_auth_method": "none"})
		case "/token":
			write(map[string]any{"access_token": "upgraded", "refresh_token": "r2", "token_type": "Bearer", "expires_in": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func approve(asked *string) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		*asked = q.Get("scope")
		cb, _ := url.Parse(q.Get("redirect_uri"))
		v := cb.Query()
		v.Set("code", "c")
		v.Set("state", q.Get("state"))
		cb.RawQuery = v.Encode()
		go func() {
			if resp, err := http.Get(cb.String()); err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

func TestStepUpAsksForWhatWasGrantedPlusWhatTheServerChallengedFor(t *testing.T) {
	var asked string
	srv := authServer(t, &asked)
	stateDir := testenv.TempDir(t)
	if err := saveMCPOAuthState(stateDir, mcpOAuthState{Version: 1, Resource: srv.URL + "/mcp", AccessToken: "old", Scope: "files:read"}); err != nil {
		t.Fatal(err)
	}
	if err := saveScopeChallenge(stateDir, scopeChallenge{Resource: srv.URL + "/mcp", Scopes: []string{"files:write", "files:read"}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	spec := Spec{Name: "files", Type: "http", URL: srv.URL + "/mcp", StateDir: stateDir, OAuthScopeUpgrade: true}
	if err := AuthorizeHTTPMCP(ctx, spec, approve(&asked)); err != nil {
		t.Fatal(err)
	}
	if asked != "files:read files:write" {
		t.Fatalf("the authorization asked for %q, want the grant plus the challenge", asked)
	}
	if got := PendingScopeUpgrade(stateDir); got != nil {
		t.Fatalf("the request is still pending after it was granted: %v", got)
	}
	if state, _ := loadMCPOAuthState(stateDir); state.AccessToken != "upgraded" {
		t.Fatalf("the new token was not saved: %+v", state)
	}
}

func TestStepUpWithNoChallengeOpensNoBrowser(t *testing.T) {
	var asked string
	srv := authServer(t, &asked)
	opened := false
	err := AuthorizeHTTPMCP(context.Background(), Spec{Name: "files", Type: "http", URL: srv.URL + "/mcp", StateDir: testenv.TempDir(t), OAuthScopeUpgrade: true},
		func(string) error { opened = true; return nil })
	if !errors.Is(err, ErrNoScopeUpgradePending) || opened {
		t.Fatalf("err=%v opened=%v", err, opened)
	}
}
