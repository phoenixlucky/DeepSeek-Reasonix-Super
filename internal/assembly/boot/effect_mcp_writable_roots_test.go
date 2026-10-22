package boot

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/ext/plugin"
)

const metaLogEnv = "REASONIX_TEST_MCP_META_LOG"

// TestMetaRecordingMCPHelper is a stdio MCP server that records the _meta of
// every tools/call it receives, one JSON object per line, in the log its
// environment names.
func TestMetaRecordingMCPHelper(t *testing.T) {
	logPath := os.Getenv(metaLogEnv)
	if logPath == "" || os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
			Params struct {
				Meta map[string]any `json:"_meta"`
			} `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "serverInfo": map[string]any{"name": "meta", "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "echo", "description": "Echo.", "inputSchema": map[string]any{"type": "object"}, "annotations": map[string]any{"readOnlyHint": true}}}}
		case "tools/call":
			if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_ = json.NewEncoder(f).Encode(req.Params.Meta)
				_ = f.Close()
			}
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}
	os.Exit(0)
}

// The folders a person let tools write beside the project reach a local stdio
// server on a real call through Build, and only those: not the project, not the
// temp directory. A session confined to its workspace tells nothing.
func TestEffectLocalMCPServerIsToldTheExtraWritableFolders(t *testing.T) {
	var rec *capabilityCallProvider
	provider.Register("boot-mcp-writable-roots", func(provider.Config) (provider.Provider, error) { return rec, nil })
	for _, tc := range []struct {
		name          string
		workspaceOnly bool
		want          bool
	}{{"configured", false, true}, {"workspace-only", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfigHome(t)
			dir := robustTempDir(t)
			extra := robustTempDir(t)
			t.Chdir(dir)
			rec = &capabilityCallProvider{direct: "mcp__meta__echo"}
			writeFile(t, dir, "reasonix.toml", `
default_model = "test-model"

[agent]
system_prompt = "BASE"

[codegraph]
enabled = false

[[providers]]
name = "test-model"
kind = "boot-mcp-writable-roots"
model = "x"
`)
			approveWorkspace(t, dir)
			logPath := filepath.Join(robustTempDir(t), "meta.jsonl")
			ctrl, err := Build(context.Background(), Options{
				Sink:           event.Discard,
				WorkspaceOnly:  tc.workspaceOnly,
				AdditionalDirs: []string{extra},
				ExtraPlugins: []plugin.Spec{{
					Name: "meta", Type: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestMetaRecordingMCPHelper$"},
					Env: map[string]string{"GO_WANT_HELPER_PROCESS": "1", metaLogEnv: logPath}, Authorized: true,
				}},
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			defer ctrl.Close()
			if err := ctrl.Run(context.Background(), "echo"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("the server saw no call: %v", err)
			}
			var meta map[string]any
			if err := json.Unmarshal(raw, &meta); err != nil {
				t.Fatalf("meta log %q: %v", raw, err)
			}
			roots, has := meta["io.reasonix/writableRoots"].([]any)
			if has != tc.want {
				t.Fatalf("writableRoots present = %v (%v), want %v; _meta %v", has, roots, tc.want, meta)
			}
			if tc.want && (len(roots) != 1 || !sameDir(roots[0].(string), extra)) {
				t.Fatalf("writableRoots = %v, want only %s", roots, extra)
			}
		})
	}
}

func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
