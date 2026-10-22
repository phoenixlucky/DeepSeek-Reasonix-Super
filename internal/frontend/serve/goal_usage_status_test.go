package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func statusBody(t *testing.T, url string) map[string]json.RawMessage {
	t.Helper()
	resp, err := http.Get(url + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStatusCarriesAGoalsUsageOnlyWhileThereIsAGoal(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	defer srv.Close()

	if _, ok := statusBody(t, srv.URL)["goalRuntime"]; ok {
		t.Fatal("a session with no goal has no usage to report")
	}
	ctrl.SetGoal("ship the release")
	raw, ok := statusBody(t, srv.URL)["goalRuntime"]
	if !ok {
		t.Fatal("a session with a goal must report its usage")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"turnsUsed", "tokensUsed", "requestsUsed", "workDurationMs", "noProgressTurns"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("goalRuntime lacks %q: %s", want, raw)
		}
	}
	for _, never := range []string{"TurnsLimit", "TokensLimit", "NoProgressLimit", "BudgetExtensions", "turnsLimit", "tokensLimit"} {
		if _, ok := fields[never]; ok {
			t.Errorf("goalRuntime carries the retired field %q", never)
		}
	}
}
