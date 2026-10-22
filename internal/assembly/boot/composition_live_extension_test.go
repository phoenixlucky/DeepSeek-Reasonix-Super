package boot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/ext/extension/sidecar"
	"reasonix/internal/ext/pluginpkg"
	"reasonix/internal/state/agentpreset"
)

func TestLiveCompositionDefersExtensionUntilInvocationAndClosesIt(t *testing.T) {
	home, root := compositionTestWorkspace(t, "openai")
	installBootFakePlugin(t, home, "lexicon", map[string]any{
		"capabilities": []string{"tools"},
		"tools":        []map[string]any{{"name": "lookup", "description": "Look up a term", "readOnly": true, "inputSchema": map[string]any{"type": "object"}}},
		"env":          map[string]string{bootFakeEnvInitResult: `{"protocolVersion":"2","name":"lexicon","version":"1.0.0","stateSchemaVersion":0,"tools":["lookup"]}`},
	})
	d := agentcomposition.Builtins()[0]
	d.ID = "user:lazy-extension"
	if _, err := agentpreset.New(home, root).Save(d, ""); err != nil {
		t.Fatal(err)
	}
	res, err := BuildRuntime(t.Context(), Options{Home: home, WorkspaceRoot: root, AgentComposition: d.ID, resolvedShell: pinnedEffectShell()})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Controller.Close()
	if res.Extensions == nil || res.Extensions.Client("lexicon") != nil {
		t.Fatal("extension was started or its lifecycle owner was lost")
	}
	before := agentcomposition.Digest(res.Assembly.Registry.Schemas())
	target, ok := res.Assembly.Registry.Get("ext__lexicon__lookup")
	if !ok {
		t.Fatal("manifest tool is missing")
	}
	result, err := target.Execute(t.Context(), json.RawMessage(`{"term":"lease"}`))
	if err != nil || !strings.Contains(result, "looked up") {
		t.Fatalf("lazy invocation: %q %v", result, err)
	}
	client := res.Extensions.Client("lexicon")
	if client == nil || client.Exited() {
		t.Fatal("invocation did not activate the runtime")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := res.Extensions.WithReady(cancelled, "lexicon", func(context.Context, *sidecar.Client) error {
		t.Error("cancelled call reached execution")
		return nil
	}); err == nil {
		t.Fatal("cancelled call was accepted")
	}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- res.Extensions.WithReady(t.Context(), "lexicon", func(_ context.Context, _ *sidecar.Client) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	parallel := make(chan error, 1)
	go func() {
		parallel <- res.Extensions.WithReady(t.Context(), "lexicon", func(_ context.Context, _ *sidecar.Client) error { return nil })
	}()
	select {
	case err := <-parallel:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("unrelated calls to one ready extension were serialized")
	}
	installed, _ := pluginpkg.LoadInstalled(home)
	entry := installed[0].Installed
	if err := pluginpkg.Upsert(home, entry); err != nil {
		close(release)
		t.Fatal(err)
	}
	next, err := res.Extensions.EnsureReady(t.Context(), "lexicon")
	if err != nil || next == client || client.Exited() {
		close(release)
		t.Fatalf("updated runtime reused or prematurely closed the in-flight instance: %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if !client.Exited() {
		t.Fatal("retired instance survived its final call")
	}
	if after := agentcomposition.Digest(res.Assembly.Registry.Schemas()); before != after {
		t.Fatal("activation changed provider-visible tools")
	}
	installed, _ = pluginpkg.LoadInstalled(home)
	entry = installed[0].Installed
	entry.Enabled = false
	if err := pluginpkg.Upsert(home, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Execute(t.Context(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("disabled extension remained callable")
	}
	proxy, _ := res.Assembly.Registry.Get("use_capability")
	if _, err := proxy.Execute(t.Context(), json.RawMessage(`{"action":"list"}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Assembly.Registry.Get(target.Name()); ok {
		t.Fatal("disabled extension remained in the capability directory")
	}
	entry.Enabled = true
	if err := pluginpkg.Upsert(home, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Execute(t.Context(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	client = res.Extensions.Client("lexicon")
	res.Controller.Close()
	if !client.Exited() {
		t.Fatal("closing the session left its deferred process running")
	}
}
