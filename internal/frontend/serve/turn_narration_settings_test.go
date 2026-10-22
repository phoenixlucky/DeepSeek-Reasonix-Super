package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func TestTurnNarrationIsBriefByDefaultAndPersistsInTheUserFile(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()

	if got := readJSON[control.TurnNarrationSettings](t, srv.URL, "/turn-narration-settings"); got.Level != config.TurnNarrationBrief {
		t.Fatalf("unset key reads %+v, want brief", got)
	}
	on := postProvider(t, srv.URL, "/turn-narration-settings", `{"level":"detailed"}`)
	on.Body.Close()
	if on.StatusCode != http.StatusOK {
		t.Fatalf("save = %d", on.StatusCode)
	}
	if got := config.LoadForEdit(config.UserConfigPath()).TurnNarration(); got != config.TurnNarrationDetailed {
		t.Fatalf("the choice did not reach [agent] turn_narration: %q", got)
	}
	if got := readJSON[control.TurnNarrationSettings](t, srv.URL, "/turn-narration-settings"); got.Level != config.TurnNarrationDetailed {
		t.Fatalf("read back %+v, want detailed", got)
	}
}

func TestTurnNarrationRefusesABodyWithoutAKnownLevel(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()
	for _, body := range []string{`{}`, `{"level":"verbose"}`, `{"level":""}`} {
		resp := postProvider(t, srv.URL, "/turn-narration-settings", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400: a missing or unknown level must not read as brief", body, resp.StatusCode)
		}
	}
}
