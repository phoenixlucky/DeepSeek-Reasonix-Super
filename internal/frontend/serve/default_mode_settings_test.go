package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func TestSaveDefaultModePersistsToTheUserFileOnly(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()

	if got := readJSON[control.DefaultModeSettings](t, srv.URL, "/default-mode"); got.Plan {
		t.Fatalf("unset choice reads %+v, want normal", got)
	}
	empty := postProvider(t, srv.URL, "/default-mode", `{}`)
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /default-mode without a choice = %d, want 400", empty.StatusCode)
	}
	resp := postProvider(t, srv.URL, "/default-mode", `{"plan":true}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /default-mode = %d", resp.StatusCode)
	}
	if !config.LoadForEdit(config.UserConfigPath()).DefaultPlanMode() {
		t.Fatal("choosing plan did not reach the user file")
	}
	back := postProvider(t, srv.URL, "/default-mode", `{"plan":false}`)
	back.Body.Close()
	if config.LoadForEdit(config.UserConfigPath()).DefaultPlanMode() {
		t.Fatal("returning to normal did not reach the user file")
	}
}

func TestSaveDefaultModeIsRefusedWithoutTheEditGrant(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()
	resp := postProvider(t, srv.URL, "/default-mode", `{"plan":true}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /default-mode without the grant = %d, want 403", resp.StatusCode)
	}
	if config.LoadForEdit(config.UserConfigPath()).DefaultPlanMode() {
		t.Fatal("a refused save reached the user file")
	}
}
