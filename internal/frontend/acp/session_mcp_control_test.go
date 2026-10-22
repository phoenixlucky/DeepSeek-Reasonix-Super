package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/session/control"
)

type controlFactory struct {
	mu   sync.Mutex
	ctrl *control.Controller
}

func (f *controlFactory) NewSession(_ context.Context, p SessionParams) (*control.Controller, error) {
	host := plugin.NewHost()
	host.SetStatusSink(p.Sink)
	runner := &fakeRunner{sink: p.Sink, behavior: func(context.Context, event.Sink, string) error { return nil }}
	ctrl := control.New(control.Options{Runner: runner, Sink: p.Sink, Host: host, WorkspaceRoot: p.Cwd})
	f.mu.Lock()
	f.ctrl = ctrl
	f.mu.Unlock()
	return ctrl, nil
}

func (f *controlFactory) controller() *control.Controller {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctrl
}

func projectWithServer(t *testing.T) string {
	t.Helper()
	cwd := testenv.TempDir(t)
	declared := `{"mcpServers":{"atlas":{"command":"project-launcher","args":["--serve"]}}}`
	if err := os.WriteFile(filepath.Join(cwd, ".mcp.json"), []byte(declared), 0o600); err != nil {
		t.Fatal(err)
	}
	return cwd
}

func TestReconnectNeverStartsAProjectServerTheUserHasNotApproved(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	factory := &controlFactory{}
	client, stop := startServer(t, factory)
	defer stop()
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1, ClientCapabilities: statusUpdatesCaps})
	id := openStatusSession(t, client, projectWithServer(t))
	waitMCPStatusUpdate(t, client)

	resp := client.call(t, sessionMCPReconnectMethod, SessionMCPNameParams{SessionID: id, Name: "atlas"})
	if resp.Error != nil {
		t.Fatalf("reconnect: %+v", resp.Error)
	}
	var out ReasonixMCPReconnect
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	if out.State != mcpControlApprovalOwed || out.Code != codeMCPApprovalOwed || !strings.Contains(out.Launch, "project-launcher") || out.Tools != 0 {
		t.Fatalf("reconnect = %+v, want approval owed with the command shown", out)
	}
	for _, row := range factory.controller().ConfiguredMCPServers() {
		if row.Entry.Name == "atlas" && (row.Enabled || !row.Pending) {
			t.Fatalf("a refused reconnect must leave the decision open: %+v", row)
		}
	}
	if failures := factory.controller().MCPServerHealth(); len(failures) != 0 && failures[0].Status == "ready" {
		t.Fatalf("the server must not have started: %+v", failures)
	}
}

func TestDisconnectIsSessionOnlyAndAnnounced(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	factory := &controlFactory{}
	client, stop := startServer(t, factory)
	defer stop()
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1, ClientCapabilities: statusUpdatesCaps})
	id := openStatusSession(t, client, projectWithServer(t))
	waitMCPStatusUpdate(t, client)

	resp := client.call(t, sessionMCPDisconnectMethod, SessionMCPNameParams{SessionID: id, Name: "atlas"})
	if resp.Error != nil {
		t.Fatalf("disconnect: %+v", resp.Error)
	}
	var out ReasonixMCPDisconnect
	if err := json.Unmarshal(resp.Result, &out); err != nil || out.Name != "atlas" || out.Disconnected {
		t.Fatalf("disconnect = %+v %v, want nothing live to disconnect", out, err)
	}
	if update := waitMCPStatusUpdate(t, client); update.SessionID != id {
		t.Fatalf("a disconnect must announce the session's MCP state: %+v", update)
	}
	for _, row := range factory.controller().ConfiguredMCPServers() {
		if row.Entry.Name == "atlas" && (row.LocalOverride || row.Enabled) {
			t.Fatalf("a session disconnect must not write the user's activation choices: %+v", row)
		}
	}
}

func TestMCPControlRefusesWhatItCannotName(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	client, stop := startServer(t, &controlFactory{})
	defer stop()
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	id := openStatusSession(t, client, testenv.TempDir(t))
	for _, method := range []string{sessionMCPReconnectMethod, sessionMCPDisconnectMethod} {
		if r := client.call(t, method, SessionMCPNameParams{SessionID: "nope", Name: "atlas"}); r.Error == nil {
			t.Fatalf("%s: an unknown session must be refused", method)
		}
		if r := client.call(t, method, SessionMCPNameParams{SessionID: id, Name: "  "}); r.Error == nil {
			t.Fatalf("%s: an empty name must be refused", method)
		}
	}
	r := client.call(t, sessionMCPReconnectMethod, SessionMCPNameParams{SessionID: id, Name: "ghost"})
	var out ReasonixMCPReconnect
	if err := json.Unmarshal(r.Result, &out); r.Error != nil || err != nil || out.State != mcpControlFailed || out.Error == "" {
		t.Fatalf("a server nobody declared fails by state: %+v %v %+v", out, err, r.Error)
	}
}

func TestExtensionFormSubmissionNeedsAnExtensionToReceiveIt(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	client, stop := startServer(t, &controlFactory{})
	defer stop()
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	id := openStatusSession(t, client, testenv.TempDir(t))
	cases := []SessionExtensionFormParams{
		{SessionID: "nope", PluginID: "p", SurfaceID: "s"},
		{SessionID: id, PluginID: " ", SurfaceID: "s"},
		{SessionID: id, PluginID: "p", SurfaceID: ""},
	}
	for _, p := range cases {
		if r := client.call(t, sessionExtensionSubmitForm, p); r.Error == nil || r.Error.Code != ErrInvalidParams {
			t.Fatalf("%+v: want invalid params, got %+v", p, r.Error)
		}
	}
	if r := client.call(t, sessionExtensionSubmitForm, SessionExtensionFormParams{SessionID: id, PluginID: "p", SurfaceID: "s"}); r.Error == nil || r.Error.Code != ErrInternal {
		t.Fatalf("with no extension running the submission cannot be accepted: %+v", r.Error)
	}
}
