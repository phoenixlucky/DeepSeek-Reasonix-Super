package boot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/state/agentpreset"
)

func TestProjectCompositionCannotChangeAnAuthorizedMCPLaunch(t *testing.T) {
	for _, kind := range []string{"args", "url"} {
		t.Run(kind, func(t *testing.T) {
			home, root := compositionTestWorkspace(t, "openai")
			marker := filepath.Join(root, "mcp-started")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			spec := plugin.Spec{Name: "authorized", Type: "stdio", Command: executable, Args: []string{"-test.run=^TestProjectCompositionMCPStartProbe$", "-test.v"}, Env: map[string]string{"COMPOSITION_MCP_PROBE_MARKER": marker}, Authorized: true}
			params := map[string]any{"args": []string{"-test.run=^TestProjectCompositionMCPStartProbe$"}}
			if kind == "url" {
				spec = plugin.Spec{Name: "authorized", Type: "http", URL: server.URL + "/original", Authorized: true}
				params = map[string]any{"url": server.URL + "/redirect"}
			}
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatal(err)
			}
			for _, scope := range []string{"project", "user"} {
				d := agentcomposition.Builtins()[0]
				d.ID = scope + ":changed-launch"
				d.MCP = agentcomposition.Selection{Mode: agentcomposition.Select, Members: []agentcomposition.Module{{ID: spec.Name, Parameters: raw}}}
				if scope == "project" {
					data, err := json.Marshal(d)
					if err != nil {
						t.Fatal(err)
					}
					writeFile(t, root, ".reasonix/agent-presets/definitions/changed-launch.json", string(data))
				} else if _, err := agentpreset.New(home, root).Save(d, ""); err != nil {
					t.Fatal(err)
				}
				built, err := BuildRuntime(t.Context(), Options{Home: home, WorkspaceRoot: root, AgentComposition: d.ID, ExtraPlugins: []plugin.Spec{spec}, resolvedShell: pinnedEffectShell()})
				if built != nil {
					proxy, ok := built.Assembly.Registry.Get("use_capability")
					if !ok {
						t.Fatal("capability proxy missing")
					}
					_, err = proxy.Execute(t.Context(), json.RawMessage(`{"action":"call","capability_id":"mcp-server:authorized"}`))
					built.Controller.Close()
				}
				_, markerErr := os.Stat(marker)
				started := requests.Load() > 0 || markerErr == nil
				if scope == "project" {
					if !errors.Is(err, agentcomposition.ErrInvalid) || started {
						t.Fatalf("project %s override reached an authorized launch: err=%v started=%t", kind, err, started)
					}
				} else if errors.Is(err, agentcomposition.ErrInvalid) || !started {
					t.Fatalf("positive control never exercised the authorized launch: %v", err)
				}
			}
		})
	}
}

func TestProjectCompositionMCPStartProbe(t *testing.T) {
	path := os.Getenv("COMPOSITION_MCP_PROBE_MARKER")
	if path == "" {
		return
	}
	if err := os.WriteFile(path, []byte("started"), 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
