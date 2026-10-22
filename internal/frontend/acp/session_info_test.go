package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/session/control"
)

type infoFactory struct{}

func (infoFactory) NewSession(_ context.Context, p SessionParams) (*control.Controller, error) {
	runner := &fakeRunner{sink: p.Sink, behavior: func(context.Context, event.Sink, string) error { return nil }}
	return control.New(control.Options{Runner: runner, Sink: p.Sink, WorkspaceRoot: p.Cwd}), nil
}

func callInfo(t *testing.T, client *rpcClient, id string) ReasonixSessionInfo {
	t.Helper()
	resp := client.call(t, sessionInfoMethod, SessionStatusParams{SessionID: id})
	if resp.Error != nil {
		t.Fatalf("session info: %+v", resp.Error)
	}
	var info ReasonixSessionInfo
	if err := json.Unmarshal(resp.Result, &info); err != nil {
		t.Fatal(err)
	}
	return info
}

func TestSessionInfoNamesTheWorkspaceAndFollowsTheApprovalMode(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	client, stop := startServer(t, infoFactory{})
	defer stop()
	resp := client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	var initialized InitializeResult
	if err := json.Unmarshal(resp.Result, &initialized); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{sessionInfoMethod, sessionMCPServersMethod} {
		capability, ok := initialized.AgentCapabilities.Meta[method].(map[string]any)
		if !ok || capability["schemaVersion"] != float64(sessionInfoSchemaVersion) {
			t.Fatalf("%s capability = %#v", method, initialized.AgentCapabilities.Meta[method])
		}
	}
	cwd := testenv.TempDir(t)
	id := openStatusSession(t, client, cwd)
	info := callInfo(t, client, id)
	if info.SessionID != id || info.SchemaVersion != sessionInfoSchemaVersion || info.AgentPreset == "" || info.Scope.Root == "" || !info.Scope.Current {
		t.Fatalf("info = %+v", info)
	}
	if info.WorkspaceRoot == "" {
		t.Fatal("the workspace the session answers for must be named")
	}
	if info.AutoApproveTools || info.Cache.HitRatio != nil {
		t.Fatalf("a new session auto-approves nothing and has no cache tally: %+v", info)
	}
	for mode, want := range map[string]bool{sessionModeLegacyAuto: true, sessionModeLegacyDefault: false} {
		if r := client.call(t, "session/set_mode", SessionSetModeParams{SessionID: id, ModeID: mode}); r.Error != nil {
			t.Fatalf("set_mode %s: %+v", mode, r.Error)
		}
		if got := callInfo(t, client, id).AutoApproveTools; got != want {
			t.Fatalf("after %s autoApproveTools = %v, want %v", mode, got, want)
		}
	}
	if r := client.call(t, sessionInfoMethod, SessionStatusParams{SessionID: "nope"}); r.Error == nil {
		t.Fatal("an unknown session must be refused")
	}
}

func TestSessionMCPServersListsDeclaredServersWithoutTheirCommands(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	cwd := testenv.TempDir(t)
	declared := `{"mcpServers":{"atlas":{"command":"secret-launcher","args":["--token","hunter2"]}}}`
	if err := os.WriteFile(filepath.Join(cwd, ".mcp.json"), []byte(declared), 0o600); err != nil {
		t.Fatal(err)
	}
	client, stop := startServer(t, infoFactory{})
	defer stop()
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	id := openStatusSession(t, client, cwd)
	resp := client.call(t, sessionMCPServersMethod, SessionMCPServersParams{SessionID: id})
	if resp.Error != nil {
		t.Fatalf("mcp servers: %+v", resp.Error)
	}
	var out ReasonixMCPServers
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Servers) != 1 || out.Servers[0].Name != "atlas" || !out.Servers[0].Pending {
		t.Fatalf("servers = %+v, want the declared one awaiting approval", out.Servers)
	}
	if body := string(resp.Result); strings.Contains(body, "secret-launcher") || strings.Contains(body, "hunter2") {
		t.Fatalf("a configured server's command must not be reported: %s", body)
	}
	one := client.call(t, sessionMCPServersMethod, SessionMCPServersParams{SessionID: id, Name: "absent"})
	var none ReasonixMCPServers
	if err := json.Unmarshal(one.Result, &none); err != nil || none.Servers == nil || len(none.Servers) != 0 {
		t.Fatalf("a name nobody declared lists nothing: %+v %v", none, err)
	}
}
