package usecap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
)

func TestLiveCompositionInventoryRefreshKeepsWhitelist(t *testing.T) {
	r := NewMCPCapabilityRuntime(t.Context(), plugin.NewHost(), nil, tool.NewRegistry(), nil)
	defer r.host.Close()
	d := agentcomposition.Builtins()[0]
	d.MCP = agentcomposition.Selection{Mode: agentcomposition.Select, Members: []agentcomposition.Module{{ID: "allowed"}}}
	r.ConfigureComposition(d)
	var current []config.PluginEntry
	r.ConfigureInventorySource(func() ([]config.PluginEntry, []plugin.Spec, map[string]bool, error) {
		var specs []plugin.Spec
		enabled := map[string]bool{}
		for _, e := range current {
			specs = append(specs, plugin.Spec{Name: e.Name, Type: "http", URL: e.URL})
			enabled[e.Name] = true
		}
		return current, specs, enabled, nil
	})
	current = []config.PluginEntry{{Name: "allowed", URL: "https://example.invalid"}, {Name: "denied", URL: "https://example.invalid"}}
	entries, _, _, _, _ := r.CapabilityCatalogState()
	if len(entries) != 1 || entries[0].Name != "allowed" {
		t.Fatalf("inventory escaped whitelist: %+v", entries)
	}
	current = nil
	entries, _, _, _, _ = r.CapabilityCatalogState()
	if len(entries) != 0 || r.serverEnabled("allowed") {
		t.Fatal("removed server remains callable")
	}
}

func TestLiveCompositionRechecksAuthorityAfterResolution(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	r := NewMCPCapabilityRuntime(t.Context(), plugin.NewHost(), nil, tool.NewRegistry(), nil)
	defer r.host.Close()
	r.ConfigureComposition(agentcomposition.Builtins()[0])
	spec := plugin.Spec{Name: "changing", Type: "http", URL: server.URL, Authorized: true, ConfigSource: "user_config"}
	r.ConfigureInventorySource(func() ([]config.PluginEntry, []plugin.Spec, map[string]bool, error) {
		return []config.PluginEntry{{Name: spec.Name}}, []plugin.Spec{spec}, map[string]bool{spec.Name: true}, nil
	})
	resolved, err := r.NewFrontend(nil, nil).ResolveCall(t.Context(), json.RawMessage(`{"action":"call","capability_id":"mcp-server:changing"}`))
	if err != nil || resolved.Target == nil {
		t.Fatalf("authorized target was not resolved: %+v %v", resolved, err)
	}
	spec.Authorized, spec.ConfigSource = false, "project_config"
	if _, err := resolved.Target.Execute(t.Context(), resolved.Args); err == nil {
		t.Fatal("stale authorization reached connection startup")
	}
	if len(r.host.ServerNames()) != 0 || requests.Load() != 0 {
		t.Fatal("revoked source established a connection")
	}
}
