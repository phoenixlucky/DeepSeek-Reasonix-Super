package boot

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/state/agentpreset"
	"reasonix/internal/state/sessionstore"
)

func TestCompositionUnselectedAndStandardPreserveWireBytesAndLegacyStorage(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		if _, err := body.ReadFrom(r.Body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		bodies = append(bodies, body.Bytes())
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"choices\":[{\"delta\":{\"content\":\"Hello.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	home, root := compositionTestWorkspace(t, "openai")
	writeFile(t, root, "reasonix.toml", fmt.Sprintf("default_model = \"test-model\"\n[[providers]]\nname=\"test-model\"\nkind=\"openai\"\nbase_url=%q\nmodel=\"x\"\n", server.URL+"/v1"))
	writeFile(t, home, "config.toml", "[agent.persona]\nprefix = 'USER PERSONA'\nsuffix = 'USER TAIL'\n")
	approveWorkspaceAt(t, home, root)
	var baseline []byte
	for _, selection := range []string{"", "builtin:general", "default"} {
		ctrl, err := Build(t.Context(), Options{Home: home, WorkspaceRoot: root, AgentComposition: selection, resolvedShell: pinnedEffectShell()})
		if err != nil {
			t.Fatal(err)
		}
		if ctrl.CompositionBinding() != nil {
			ctrl.Close()
			t.Fatalf("%q implicitly bound a session", selection)
		}
		mu.Lock()
		start := len(bodies)
		mu.Unlock()
		ctrl.EnsureSessionPath()
		err = ctrl.RunTurn(t.Context(), "Say hello.")
		if err != nil {
			ctrl.Close()
			t.Fatal(err)
		}
		if err := ctrl.Snapshot(); err != nil {
			ctrl.Close()
			t.Fatal(err)
		}
		log, err := os.ReadFile(sessionstore.SessionEventLogPath(ctrl.SessionPath()))
		ctrl.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(log, []byte(`"schema_version":4`)) || bytes.Contains(log, []byte(`agent_composition`)) {
			t.Fatal("ordinary session changed storage format")
		}
		mu.Lock()
		sent := append([]byte(nil), bodies[start]...)
		mu.Unlock()
		if !bytes.Contains(sent, []byte("USER PERSONA")) {
			t.Fatal("user persona was lost")
		}
		if baseline == nil {
			baseline = sent
		} else if !bytes.Equal(baseline, sent) {
			t.Fatalf("%q changed provider request bytes: %s", selection, firstDivergence(string(baseline), string(sent)))
		}
	}
	store := agentpreset.New(home, root)
	if _, err := os.Stat(store.UserDirectory()); !os.IsNotExist(err) {
		t.Fatalf("unselected boot migrated the persona or wrote presets: %v", err)
	}
	if err := store.SetDefault("builtin:minimal", false); err != nil {
		t.Fatal(err)
	}
	ctrl, err := Build(t.Context(), Options{Home: home, WorkspaceRoot: root, resolvedShell: pinnedEffectShell()})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	if ctrl.CompositionBinding() != nil {
		t.Fatal("stored preference implicitly bound an unselected session")
	}
	if _, err := ctrl.AgentPresets(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.UserDirectory(), "definitions")); !os.IsNotExist(err) {
		t.Fatalf("catalog read migrated persona: %v", err)
	}
	if len(agentcomposition.Builtins()) != 2 {
		t.Fatal("unexpected built-in catalog")
	}
}
