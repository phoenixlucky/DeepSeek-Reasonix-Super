package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func TestSavePlanApprovalPersistsToTheUserFileOnly(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()

	if got := readJSON[control.PlanApprovalSettings](t, srv.URL, "/plan-approval"); got.Auto {
		t.Fatalf("unset choice reads %+v, want ask", got)
	}
	empty := postProvider(t, srv.URL, "/plan-approval", `{}`)
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /plan-approval without a choice = %d, want 400", empty.StatusCode)
	}
	resp := postProvider(t, srv.URL, "/plan-approval", `{"auto":true}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /plan-approval = %d", resp.StatusCode)
	}
	if !config.LoadForEdit(config.UserConfigPath()).PlanApprovalAuto() {
		t.Fatal("choosing auto did not reach the user file")
	}
	back := postProvider(t, srv.URL, "/plan-approval", `{"auto":false}`)
	back.Body.Close()
	if config.LoadForEdit(config.UserConfigPath()).PlanApprovalAuto() {
		t.Fatal("returning to ask did not reach the user file")
	}
}
