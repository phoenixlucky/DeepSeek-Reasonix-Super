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

func followServer(t *testing.T, allowEdit bool) string {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	body := "[[plugins]]\nname = \"figma\"\ntype = \"http\"\nurl = \"https://mcp.example.test/mcp\"\n"
	if err := os.WriteFile(config.UserConfigPath(), []byte(body), 0o600); err != nil {
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

func TestMCPFollowRouteValidatesAndNeedsTheGrant(t *testing.T) {
	url := followServer(t, true)
	if status, _ := postSide(t, url+"/mcp/follow", map[string]any{"follow": true}); status != http.StatusBadRequest {
		t.Fatalf("no name = %d", status)
	}
	if status, _ := postSide(t, url+"/mcp/follow", map[string]any{"name": "figma"}); status != http.StatusBadRequest {
		t.Fatalf("no choice = %d", status)
	}
	if status, _ := postSide(t, url+"/mcp/follow", map[string]any{"name": "nobody", "follow": true}); status != http.StatusNotFound {
		t.Fatalf("unknown server = %d", status)
	}

	closed := followServer(t, false)
	status, raw := postSide(t, closed+"/mcp/follow", map[string]any{"name": "figma", "follow": true})
	if status != http.StatusForbidden || !strings.Contains(string(raw), "mcp.editing_disabled") {
		t.Fatalf("without the grant = %d %s", status, raw)
	}
}
