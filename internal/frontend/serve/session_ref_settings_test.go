package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func TestSessionRefLimitStartsAtTheDefaultAndPersistsInTheUserFile(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()

	got := readJSON[control.SessionRefSettings](t, srv.URL, "/session-ref-settings")
	if got.Tokens != config.DefaultSessionRefTokens || got.MinTokens != config.SessionRefTokensMin || got.MaxTokens != config.SessionRefTokensMax {
		t.Fatalf("unset reads %+v, want the default inside its bounds", got)
	}
	ok := postProvider(t, srv.URL, "/session-ref-settings", `{"tokens":12000}`)
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("save = %d", ok.StatusCode)
	}
	if file := config.LoadForEdit(config.UserConfigPath()).Desktop.SessionRefTokens; file != 12000 {
		t.Fatalf("file holds %d, want 12000", file)
	}
	if again := readJSON[control.SessionRefSettings](t, srv.URL, "/session-ref-settings"); again.Tokens != 12000 {
		t.Fatalf("after a save the setting reads %d", again.Tokens)
	}
}

func TestSessionRefLimitRefusesWhatIsOutOfRangeAndABodyWithoutTheNumber(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()

	bad := postProvider(t, srv.URL, "/session-ref-settings", `{"tokens":50}`)
	var refusal struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(bad.Body).Decode(&refusal)
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest || refusal.Code != "session_ref.out_of_range" {
		t.Fatalf("50 tokens = %d %q, want 400 session_ref.out_of_range", bad.StatusCode, refusal.Code)
	}
	empty := postProvider(t, srv.URL, "/session-ref-settings", `{}`)
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body = %d, want 400: a missing number must not read as the default", empty.StatusCode)
	}
	if config.LoadForEdit(config.UserConfigPath()).Desktop.SessionRefTokens != 0 {
		t.Fatal("a refused save reached the file")
	}
}
