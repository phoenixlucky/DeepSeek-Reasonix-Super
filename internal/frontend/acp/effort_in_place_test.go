package acp

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
)

// requestEffortStub is a provider that takes the given depths on each request.
type requestEffortStub struct{ levels []string }

func (requestEffortStub) Name() string               { return "stub" }
func (s requestEffortStub) RequestEfforts() []string { return s.levels }
func (requestEffortStub) Stream(context.Context, provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk)
	close(ch)
	return ch, nil
}

// A depth the provider takes per request is applied without a rebuild, and the
// config option reports it; a depth it does not take still rebuilds.
func TestSessionConfigAppliesAnEffortInPlaceWhenTheProviderTakesItPerRequest(t *testing.T) {
	factory := &configurableFactory{requestEfforts: []string{"high"}}
	client, stop := startServer(t, factory)
	defer stop()

	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	newResp := client.call(t, "session/new", SessionNewParams{Cwd: testenv.TempDir(t)})
	var nr SessionNewResult
	if err := json.Unmarshal(newResp.Result, &nr); err != nil {
		t.Fatalf("session/new result: %v", err)
	}
	resp := client.call(t, "session/set_config_option", SetSessionConfigOptionParams{SessionID: nr.SessionID, ConfigID: "effort", Value: "high"})
	if resp.Error != nil {
		t.Fatalf("set effort: %+v", resp.Error)
	}
	var set SetSessionConfigOptionResult
	if err := json.Unmarshal(resp.Result, &set); err != nil {
		t.Fatal(err)
	}
	if opt, _ := findConfigOption(set.ConfigOptions, "effort"); opt.CurrentValue != "high" {
		t.Fatalf("effort option after the change = %q, want high", opt.CurrentValue)
	}
	if got := factory.buildCount(); got != 1 {
		t.Fatalf("the depth change rebuilt the session (%d builds), want the initial build only", got)
	}
}

// Without the capability the same request is the rebuild it always was.
func TestSessionConfigStillRebuildsAnEffortTheProviderDoesNotTakePerRequest(t *testing.T) {
	factory := &configurableFactory{}
	client, stop := startServer(t, factory)
	defer stop()

	client.call(t, "initialize", InitializeParams{ProtocolVersion: 1})
	newResp := client.call(t, "session/new", SessionNewParams{Cwd: testenv.TempDir(t)})
	var nr SessionNewResult
	if err := json.Unmarshal(newResp.Result, &nr); err != nil {
		t.Fatalf("session/new result: %v", err)
	}
	if resp := client.call(t, "session/set_config_option", SetSessionConfigOptionParams{SessionID: nr.SessionID, ConfigID: "effort", Value: "high"}); resp.Error != nil {
		t.Fatalf("set effort: %+v", resp.Error)
	}
	if got := factory.buildCount(); got != 2 {
		t.Fatalf("builds = %d, want the initial one and one rebuild", got)
	}
}
