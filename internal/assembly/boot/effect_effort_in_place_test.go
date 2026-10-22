package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/session/control"
)

// A depth change on a provider that takes one per request reaches the wire on the
// next call of the same controller: nothing is rebuilt, the running selection
// follows, and a depth the provider cannot take per request stays a rebuild.
func TestEffectEffortChangesInPlaceOnARequestScopedProvider(t *testing.T) {
	isolateConfigHome(t)
	rememberHome(t)
	fenceBootTestHistoryCatalog(t)
	t.Setenv("INPLACE_FAKE_KEY", "fixture-key")
	dir := robustTempDir(t)
	t.Chdir(dir)
	var mu sync.Mutex
	var sent []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		sent = append(sent, body["reasoning_effort"])
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, chatCompletionsSSE)
	}))
	defer srv.Close()
	writeFile(t, dir, "reasonix.toml", fmt.Sprintf(`
default_model = "relay/parent"
[codegraph]
enabled = false
[[providers]]
name = "relay"
kind = "openai"
base_url = "%s"
api_key_env = "INPLACE_FAKE_KEY"
models = ["parent"]
supported_efforts = ["low", "medium", "high"]
default_effort = "low"
`, srv.URL))
	approveWorkspace(t, dir)
	ctrl, err := Build(context.Background(), Options{Model: "relay/parent", WorkspaceRoot: dir, MaxSteps: 3, Sink: event.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	ask := func() {
		t.Helper()
		if err := ctrl.Run(context.Background(), "say ok"); err != nil {
			t.Fatal(err)
		}
	}
	last := func() any {
		mu.Lock()
		defer mu.Unlock()
		return sent[len(sent)-1]
	}

	before := ctrl.RuntimeSelection()
	ask()
	if got := last(); got != "low" {
		t.Fatalf("first request carried %v, want the configured low", got)
	}
	high := control.RuntimeSelection{ModelRef: before.ModelRef, Effort: "high", ProviderFingerprint: "fp-high"}
	if !ctrl.EffortInPlaceEligible("high") || !ctrl.SetEffortInPlace("high", high) {
		t.Fatal("a depth the provider takes per request was not applied in place")
	}
	if got := ctrl.RuntimeSelection(); got.Effort != "high" || !ctrl.MatchesRuntimeSelection(high) || ctrl.MatchesRuntimeSelection(before) {
		t.Fatalf("running selection %+v does not follow the change (was %+v)", got, before)
	}
	if face, ok := ctrl.ModelFace(); !ok || face.Effort != "high" {
		t.Fatalf("the status line would still read %q after the change, want high", face.Effort)
	}
	ask()
	if got := last(); got != "high" {
		t.Fatalf("request after the change carried %v, want high", got)
	}
	if !ctrl.SetEffortInPlace("low", before) {
		t.Fatal("going back to the built depth was refused")
	}
	ask()
	if got := last(); got != "low" {
		t.Fatalf("request after going back carried %v, want low", got)
	}
	for _, level := range []string{"auto", "max", ""} {
		if ctrl.EffortInPlaceEligible(level) || ctrl.SetEffortInPlace(level, before) {
			t.Fatalf("%q is not a request-scoped depth for this provider but was applied in place", level)
		}
	}
}
