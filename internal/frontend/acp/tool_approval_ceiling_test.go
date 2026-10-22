package acp

import (
	"encoding/json"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/session/control"
	"reasonix/internal/state/sessionstore"
)

type cappedFactory struct {
	configurableFactory
	ceiling string
}

func (f *cappedFactory) PostureCeiling() string { return f.ceiling }

func openCapped(t *testing.T, ceiling string) (*rpcClient, func(), SessionNewResult) {
	t.Helper()
	client, stop := startServer(t, &cappedFactory{ceiling: ceiling})
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	resp := client.call(t, "session/new", SessionNewParams{Cwd: testenv.TempDir(t)})
	var nr SessionNewResult
	if err := json.Unmarshal(resp.Result, &nr); err != nil {
		t.Fatalf("session/new result: %v (%+v)", err, resp.Error)
	}
	return client, stop, nr
}

func setApproval(t *testing.T, c *rpcClient, id, value string) frame {
	t.Helper()
	return c.call(t, "session/set_config_option", SetSessionConfigOptionParams{SessionID: id, ConfigID: "tool_approval", Value: value})
}

func ceilingCode(t *testing.T, f frame) ToolApprovalData {
	t.Helper()
	if f.Error == nil || f.Error.Code != ErrInvalidParams {
		t.Fatalf("want an invalid-params refusal, got %+v", f.Error)
	}
	var data ToolApprovalData
	if err := json.Unmarshal(f.Error.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func approvalValues(o SessionConfigOption) []string {
	var out []string
	for _, v := range o.Options {
		out = append(out, v.Value)
	}
	return out
}

func TestPostureCeilingStartsThereAndRefusesRaising(t *testing.T) {
	client, stop, nr := openCapped(t, control.ToolApprovalAuto)
	defer stop()
	opt, _ := findConfigOption(nr.ConfigOptions, "tool_approval")
	if opt.CurrentValue != control.ToolApprovalAuto {
		t.Fatalf("starts in %q, want auto", opt.CurrentValue)
	}
	for _, v := range approvalValues(opt) {
		if v == control.ToolApprovalYolo {
			t.Fatalf("the options offer %q above the ceiling: %v", v, approvalValues(opt))
		}
	}
	data := ceilingCode(t, setApproval(t, client, nr.SessionID, control.ToolApprovalYolo))
	if data.Code != codeToolApprovalAboveCeiling || data.Ceiling != control.ToolApprovalAuto {
		t.Fatalf("refusal data = %+v", data)
	}
	if r := setApproval(t, client, nr.SessionID, control.ToolApprovalReadOnly); r.Error != nil {
		t.Fatalf("lowering to read-only refused: %+v", r.Error)
	}
	if r := setApproval(t, client, nr.SessionID, control.ToolApprovalAuto); r.Error != nil {
		t.Fatalf("returning to the ceiling refused: %+v", r.Error)
	}
	ceilingCode(t, setApproval(t, client, nr.SessionID, control.ToolApprovalYolo))
}

func TestPostureCeilingGuardsLegacySetMode(t *testing.T) {
	client, stop, nr := openCapped(t, control.ToolApprovalAuto)
	defer stop()
	data := ceilingCode(t, client.call(t, "session/set_mode", SessionSetModeParams{SessionID: nr.SessionID, ModeID: sessionModeLegacyAuto}))
	if data.Code != codeToolApprovalAboveCeiling {
		t.Fatalf("legacy auto above an auto ceiling: %+v", data)
	}
	if r := client.call(t, "session/set_mode", SessionSetModeParams{SessionID: nr.SessionID, ModeID: sessionModeLegacyDefault}); r.Error != nil {
		t.Fatalf("legacy default (ask) is below the ceiling: %+v", r.Error)
	}

	client2, stop2, nr2 := openCapped(t, control.ToolApprovalReadOnly)
	defer stop2()
	ceilingCode(t, client2.call(t, "session/set_mode", SessionSetModeParams{SessionID: nr2.SessionID, ModeID: sessionModeLegacyDefault}))
}

func TestNoCeilingKeepsTheOpenPosture(t *testing.T) {
	client, stop, nr := openCapped(t, "")
	defer stop()
	opt, _ := findConfigOption(nr.ConfigOptions, "tool_approval")
	if opt.CurrentValue != control.ToolApprovalAsk || len(opt.Options) != 3 {
		t.Fatalf("uncapped option = %+v, want ask with ask/auto/yolo", opt)
	}
	if r := setApproval(t, client, nr.SessionID, control.ToolApprovalYolo); r.Error != nil {
		t.Fatalf("uncapped yolo refused: %+v", r.Error)
	}
}

func seedSavedPosture(t *testing.T, dir, id, mode string) string {
	t.Helper()
	path := transcriptPath(dir, id)
	saved := sessionstore.NewSession("")
	saved.Add(provider.Message{Role: provider.RoleUser, Content: "hello"})
	if err := saved.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := saveACPMeta(path, acpSessionMeta{
		SessionID: id, Cwd: dir, ToolApprovalMode: mode, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPostureCeilingClampsSavedPostureWithoutRewritingIt(t *testing.T) {
	dir := testenv.TempDir(t)
	path := seedSavedPosture(t, dir, "clamp", control.ToolApprovalYolo)
	factory := &cappedFactory{configurableFactory: configurableFactory{dir: dir}, ceiling: control.ToolApprovalAuto}
	client, stop := startServer(t, factory)
	defer stop()
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	resp := client.call(t, "session/load", SessionLoadParams{SessionID: "clamp", Cwd: dir})
	if resp.Error != nil {
		t.Fatalf("session/load: %+v", resp.Error)
	}
	var lr SessionLoadResult
	if err := json.Unmarshal(resp.Result, &lr); err != nil {
		t.Fatal(err)
	}
	if opt, _ := findConfigOption(lr.ConfigOptions, "tool_approval"); opt.CurrentValue != control.ToolApprovalAuto {
		t.Fatalf("a saved yolo opened as %q under an auto ceiling", opt.CurrentValue)
	}
	meta, _, err := loadACPMeta(path)
	if err != nil || meta.ToolApprovalMode != control.ToolApprovalYolo {
		t.Fatalf("the file now says %q (%v); the clamp must not rewrite it", meta.ToolApprovalMode, err)
	}
	if r := setApproval(t, client, "clamp", control.ToolApprovalAsk); r.Error != nil {
		t.Fatalf("lowering refused: %+v", r.Error)
	}
	if meta, _, _ = loadACPMeta(path); meta.ToolApprovalMode != control.ToolApprovalAsk {
		t.Fatalf("an explicit choice left the file at %q, want ask", meta.ToolApprovalMode)
	}
}

func TestUncappedLoadKeepsSavedReadOnly(t *testing.T) {
	dir := testenv.TempDir(t)
	seedSavedPosture(t, dir, "ro", control.ToolApprovalReadOnly)
	client, stop := startServer(t, &cappedFactory{configurableFactory: configurableFactory{dir: dir}})
	defer stop()
	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	resp := client.call(t, "session/load", SessionLoadParams{SessionID: "ro", Cwd: dir})
	var lr SessionLoadResult
	if err := json.Unmarshal(resp.Result, &lr); err != nil {
		t.Fatalf("%v %+v", err, resp.Error)
	}
	if opt, _ := findConfigOption(lr.ConfigOptions, "tool_approval"); opt.CurrentValue != control.ToolApprovalReadOnly {
		t.Fatalf("a saved read-only opened as %q", opt.CurrentValue)
	}
}
