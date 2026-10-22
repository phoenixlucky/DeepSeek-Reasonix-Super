package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func signInServer(t *testing.T, allowEdit bool, pluginsTOML string) string {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	if err := os.WriteFile(config.UserConfigPath(), []byte(pluginsTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := testenv.TempDir(t)
	path := filepath.Join(dir, "sessions", "me.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, Label: "me", ModelRef: config.UnconfiguredRef, WorkspaceRoot: dir, SessionDir: filepath.Dir(path), SessionPath: path})
	t.Cleanup(ctrl.Close)
	server := New(ctrl, bc, config.ServeConfig{})
	if allowEdit {
		server.AllowProviderEdit()
	}
	srv := httptest.NewServer(operatorHandler(server))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestMCPSignInRouteRefusalsCarryTheirOwnCodes(t *testing.T) {
	withKey := "[[plugins]]\nname = \"keyed\"\ntype = \"http\"\nurl = \"https://mcp.example.test/mcp\"\n[plugins.headers]\nAuthorization = \"Bearer x\"\n"
	url := signInServer(t, true, withKey)
	cases := []struct {
		body   map[string]any
		status int
		code   string
	}{
		{map[string]any{"name": "keyed"}, http.StatusBadRequest, "mcp.signin_ineligible"},
		{map[string]any{"name": "nobody"}, http.StatusNotFound, ""},
		{map[string]any{}, http.StatusBadRequest, ""},
	}
	for _, c := range cases {
		status, raw := postSide(t, url+"/mcp/signin", c.body)
		if status != c.status || !strings.Contains(string(raw), c.code) {
			t.Fatalf("%v = %d %s", c.body, status, raw)
		}
	}
}

func TestANetworkedClientCannotStartAMCPSignIn(t *testing.T) {
	url := signInServer(t, false, "[[plugins]]\nname = \"figma\"\ntype = \"http\"\nurl = \"https://mcp.example.test/mcp\"\n")
	status, raw := postSide(t, url+"/mcp/signin", map[string]any{"name": "figma"})
	if status != http.StatusForbidden || !strings.Contains(string(raw), "mcp.editing_disabled") {
		t.Fatalf("without the grant = %d %s", status, raw)
	}
}

func TestMCPUpgradeRouteNeedsTheGrantAndAnAskingServer(t *testing.T) {
	body := "[[plugins]]\nname = \"figma\"\ntype = \"http\"\nurl = \"https://mcp.example.test/mcp\"\n"
	status, raw := postSide(t, signInServer(t, false, body)+"/mcp/upgrade", map[string]any{"name": "figma"})
	if status != http.StatusForbidden || !strings.Contains(string(raw), "mcp.editing_disabled") {
		t.Fatalf("without the grant = %d %s", status, raw)
	}
	status, raw = postSide(t, signInServer(t, true, body)+"/mcp/upgrade", map[string]any{"name": "figma"})
	if status != http.StatusConflict || !strings.Contains(string(raw), "mcp.no_scope_upgrade") {
		t.Fatalf("nothing asked = %d %s", status, raw)
	}
}
