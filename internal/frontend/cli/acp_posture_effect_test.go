package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
	"reasonix/internal/frontend/acp"
)

const acpWriterKind = "acp-posture-writer"

// acpWriter asks for one write_file at the target the test names, and keeps the
// tool result the model was handed, so a refusal is read as the model saw it.
type acpWriter struct {
	mu      sync.Mutex
	target  string
	results []string
}

var acpWriterCur acpWriter

func init() {
	provider.Register(acpWriterKind, func(cfg provider.Config) (provider.Provider, error) {
		return &acpWriterProvider{name: cfg.Name}, nil
	})
}

type acpWriterProvider struct{ name string }

func (p *acpWriterProvider) Name() string { return p.name }

func (p *acpWriterProvider) Stream(_ context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk, 3)
	last := req.Messages[len(req.Messages)-1]
	acpWriterCur.mu.Lock()
	target := acpWriterCur.target
	if last.Role == provider.RoleTool {
		acpWriterCur.results = append(acpWriterCur.results, last.Content)
	}
	acpWriterCur.mu.Unlock()
	if last.Role == provider.RoleUser && strings.Contains(last.Content, "write-it") {
		args := fmt.Sprintf(`{"path":%q,"content":"x"}`, target)
		ch <- provider.Chunk{Type: provider.ChunkToolCall, ToolCall: &provider.ToolCall{ID: "w-1", Name: "write_file", Arguments: args}}
	} else {
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "done"}
	}
	ch <- provider.Chunk{Type: provider.ChunkDone}
	close(ch)
	return ch, nil
}

func (w *acpWriter) set(target string) {
	w.mu.Lock()
	w.target, w.results = target, nil
	w.mu.Unlock()
}

func (w *acpWriter) toolResult() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.results, "\n")
}

type wireFrame struct {
	ID     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Result json.RawMessage  `json:"result"`
	Error  *struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	} `json:"error"`
}

// acpWire is a client on the far end of acp.Serve. It answers every permission
// request with a cancel and counts them, so "no card was raised" is observable.
type acpWire struct {
	in    io.Writer
	mu    sync.Mutex
	next  int
	waits map[int]chan wireFrame
	cards atomic.Int32
}

func serveACP(t *testing.T, factory *acpFactory) *acpWire {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan struct{})
	go func() {
		_ = acp.Serve(context.Background(), inR, outW, factory, acp.AgentInfo{Name: "reasonix-test", Version: "0"})
		close(done)
	}()
	w := &acpWire{in: inW, waits: map[int]chan wireFrame{}}
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var f wireFrame
			if json.Unmarshal(sc.Bytes(), &f) != nil {
				continue
			}
			switch {
			case f.Method == "session/request_permission" && f.ID != nil:
				w.cards.Add(1)
				w.write(map[string]any{"jsonrpc": "2.0", "id": f.ID, "result": map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}})
			case f.Method == "" && f.ID != nil:
				var id int
				if json.Unmarshal(*f.ID, &id) == nil {
					w.mu.Lock()
					ch := w.waits[id]
					w.mu.Unlock()
					if ch != nil {
						ch <- f
					}
				}
			}
		}
	}()
	t.Cleanup(func() {
		_ = inW.Close()
		<-done
		_ = outW.Close()
	})
	return w
}

func (w *acpWire) write(v any) {
	b, _ := json.Marshal(v)
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.in.Write(append(b, '\n'))
}

func (w *acpWire) call(t *testing.T, method string, params any) wireFrame {
	t.Helper()
	w.mu.Lock()
	w.next++
	id := w.next
	ch := make(chan wireFrame, 1)
	w.waits[id] = ch
	w.mu.Unlock()
	w.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	select {
	case f := <-ch:
		return f
	case <-time.After(testenv.Budget(t)):
		t.Fatalf("%s: timed out", method)
		return wireFrame{}
	}
}

func (w *acpWire) open(t *testing.T, cwd string) string {
	t.Helper()
	w.call(t, "initialize", acp.InitializeParams{ProtocolVersion: 1})
	f := w.call(t, "session/new", acp.SessionNewParams{Cwd: cwd})
	var nr acp.SessionNewResult
	if err := json.Unmarshal(f.Result, &nr); err != nil {
		t.Fatalf("session/new: %v (%+v)", err, f.Error)
	}
	return nr.SessionID
}

func (w *acpWire) prompt(t *testing.T, id string) {
	t.Helper()
	f := w.call(t, "session/prompt", acp.SessionPromptParams{SessionID: id, Prompt: []acp.ContentBlock{{Type: "text", Text: "write-it"}}})
	if f.Error != nil {
		t.Fatalf("session/prompt: %+v", f.Error)
	}
}

func (w *acpWire) setApproval(t *testing.T, id, value string) wireFrame {
	t.Helper()
	return w.call(t, "session/set_config_option", acp.SetSessionConfigOptionParams{SessionID: id, ConfigID: "tool_approval", Value: value})
}

// acpPostureWorld is a trusted project with the writer provider, and a folder
// beside the package that is outside every write root.
func acpPostureWorld(t *testing.T, extraToml string) (project, outside string) {
	t.Helper()
	pkg, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if outside, err = os.MkdirTemp(pkg, "posture-outside-*"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })
	isolateCLIConfigHome(t)
	if _, err := config.SetCredential("REASONIX_TEST_KEY", "test-key"); err != nil {
		t.Fatal(err)
	}
	project = testenv.TempDir(t)
	toml := `
default_model = "local"

[codegraph]
enabled = false

[[providers]]
name = "local"
kind = "` + acpWriterKind + `"
base_url = "http://example.invalid"
model = "fake-model"
api_key_env = "REASONIX_TEST_KEY"
` + extraToml
	if err := os.WriteFile(filepath.Join(project, "reasonix.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	approveWorkspace(t, project)
	return project, outside
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestACPPermissionModeAutoWritesInsideWithoutACardAndStaysCapped(t *testing.T) {
	project, outside := acpPostureWorld(t, "")
	w := serveACP(t, &acpFactory{postureCeiling: "auto"})
	id := w.open(t, project)

	inside := filepath.Join(project, "inside.txt")
	acpWriterCur.set(inside)
	w.prompt(t, id)
	if !exists(inside) || w.cards.Load() != 0 {
		t.Fatalf("auto: file written=%v, cards=%d; want a write with no card", exists(inside), w.cards.Load())
	}

	target := filepath.Join(outside, "out.txt")
	acpWriterCur.set(target)
	w.prompt(t, id)
	if exists(target) {
		t.Fatalf("auto wrote outside the workspace: %s", acpWriterCur.toolResult())
	}

	raise := w.setApproval(t, id, "yolo")
	if raise.Error == nil || !strings.Contains(string(raise.Error.Data), "acp.tool_approval.above_ceiling") {
		t.Fatalf("raising to yolo = %+v, want the typed above-ceiling refusal", raise.Error)
	}
	acpWriterCur.set(target)
	w.prompt(t, id)
	if exists(target) {
		t.Fatalf("a refused raise still opened the outside write: %s", acpWriterCur.toolResult())
	}
}

func TestACPPermissionModeReadOnlyRefusesWritesAndAProjectFileCannotLoosenIt(t *testing.T) {
	project, _ := acpPostureWorld(t, "\n[desktop]\ndefault_tool_approval_mode = \"yolo\"\n")
	w := serveACP(t, &acpFactory{postureCeiling: "readOnly"})
	id := w.open(t, project)
	inside := filepath.Join(project, "inside.txt")
	acpWriterCur.set(inside)
	w.prompt(t, id)
	if exists(inside) || w.cards.Load() != 0 {
		t.Fatalf("read-only: file written=%v, cards=%d; want a refusal that asked nobody: %s", exists(inside), w.cards.Load(), acpWriterCur.toolResult())
	}
	if r := w.setApproval(t, id, "auto"); r.Error == nil {
		t.Fatal("read-only accepted a raise to auto")
	}
}

func TestACPWithoutPermissionModeStaysAsk(t *testing.T) {
	project, _ := acpPostureWorld(t, "")
	w := serveACP(t, &acpFactory{})
	id := w.open(t, project)
	acpWriterCur.set(filepath.Join(project, "inside.txt"))
	w.prompt(t, id)
	if w.cards.Load() == 0 {
		t.Fatal("the default posture raised no approval card for a write")
	}
	if r := w.setApproval(t, id, "yolo"); r.Error != nil {
		t.Fatalf("an uncapped agent refused yolo: %+v", r.Error)
	}
}

func TestACPPermissionModeFlagParsing(t *testing.T) {
	isolateCLIConfigHome(t)
	for _, bad := range []string{"plan", "nonsense"} {
		var rc int
		_ = captureStderr(t, func() { rc = acpCommand([]string{"--permission-mode", bad}, "test") })
		if rc != 2 {
			t.Fatalf("--permission-mode %s exited %d, want 2", bad, rc)
		}
	}
}

func TestACPWorkspaceOnlyRefusesAClientsAdditionalDirectories(t *testing.T) {
	project, outside := acpPostureWorld(t, "")
	w := serveACP(t, &acpFactory{postureCeiling: "auto", workspaceOnly: true})
	w.call(t, "initialize", acp.InitializeParams{ProtocolVersion: 1})
	f := w.call(t, "session/new", acp.SessionNewParams{Cwd: project, AdditionalDirectories: []string{outside}})
	if f.Error == nil || !strings.Contains(string(f.Error.Data), "acp.additional_directories.disabled") {
		t.Fatalf("session/new with an extra folder under --workspace-only = %+v, want the typed refusal", f.Error)
	}
	id := w.open(t, project)
	target := filepath.Join(outside, "out.txt")
	acpWriterCur.set(target)
	w.prompt(t, id)
	if exists(target) {
		t.Fatalf("a --workspace-only agent wrote into the folder the client asked for: %s", acpWriterCur.toolResult())
	}
}
