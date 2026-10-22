package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/session/control"
	"reasonix/internal/state/sessionstore"
)

// holdingEffortProvider takes a depth per request and holds its reply until the
// test lets it go, so the session is mid-turn while the depth changes.
type holdingEffortProvider struct{ release chan struct{} }

func (p *holdingEffortProvider) Name() string             { return "alternate" }
func (p *holdingEffortProvider) RequestEfforts() []string { return []string{"low", "high"} }
func (p *holdingEffortProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Chunk, error) {
	ch := make(chan provider.Chunk, 2)
	go func() {
		defer close(ch)
		select {
		case <-p.release:
		case <-ctx.Done():
			return
		}
		ch <- provider.Chunk{Type: provider.ChunkText, Text: "ok"}
		ch <- provider.Chunk{Type: provider.ChunkDone}
	}()
	return ch, nil
}

// A depth the provider takes per request is applied to the running session: no
// rebuild, no refusal while a turn is in flight, the running selection follows,
// and the next request carries it.
func TestEffortChangesInPlaceWhileATurnRuns(t *testing.T) {
	writeServeEffortSelectionConfig(t, "low")
	bc := NewBroadcaster()
	low := serveProviderIdentity(t, "alternate/shared-chat")
	p := &holdingEffortProvider{release: make(chan struct{})}
	exec := agent.New(p, tool.NewRegistry(), sessionstore.NewSession("sys"), agent.Options{}, event.Discard)
	ctrl := control.New(control.Options{
		Runner: exec, Executor: exec, Sink: bc, ModelRef: "alternate/shared-chat", Effort: "low",
		ProviderFingerprint: low.Fingerprint, SessionDir: testenv.TempDir(t),
	})
	defer ctrl.Close()
	server := New(ctrl, bc, config.ServeConfig{})
	built := 0
	server.buildController = func(context.Context, string) (*control.Controller, error) {
		built++
		return control.New(control.Options{Sink: bc}), nil
	}
	srv := httptest.NewServer(operatorHandler(server))
	defer srv.Close()

	ctrl.SubmitHTTP("think about it")
	waitRunning(t, ctrl)
	resp, err := http.Post(srv.URL+"/effort", "application/json", strings.NewReader(`{"effort":"high"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /effort mid-turn = %d, want 204 (a depth change that needs no rebuild is not refused for being busy)", resp.StatusCode)
	}
	if built != 0 {
		t.Fatalf("the depth change rebuilt the controller %d times", built)
	}
	if got := ctrl.RuntimeSelection().Effort; got != "high" {
		t.Fatalf("running selection effort = %q, want high", got)
	}
	if got := exec.RequestEffort(); got != "high" {
		t.Fatalf("the next request would carry %q, want high", got)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := cfg.ResolveModel("alternate/shared-chat")
	if entry == nil || entry.Effort != "high" {
		t.Fatalf("persisted effort = %+v, want high so a restart opens on it", entry)
	}
	close(p.release)
	waitNotRunning(t, ctrl)
}
