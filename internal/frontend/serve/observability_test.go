package serve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func TestObservabilitySettingsAreOffByDefaultAndPersistInTheUserFile(t *testing.T) {
	t.Setenv("OTEL_SERVE_AUTH", "x")
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()
	if got := readJSON[control.ObservabilitySettings](t, srv.URL, "/observability"); got.Enabled || got.Endpoint != "" || got.Applies != "next_session" {
		t.Fatalf("default = %+v", got)
	}
	resp := postProvider(t, srv.URL, "/observability", `{"enabled":true,"endpoint":"http://localhost:4318","headerName":"Authorization","headerEnv":"OTEL_SERVE_AUTH","serviceName":"mine"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save = %d", resp.StatusCode)
	}
	if got := config.LoadForEdit(config.UserConfigPath()).Observability.OTLP; got != (config.OTLPConfig{Enabled: true, Endpoint: "http://localhost:4318", HeaderName: "Authorization", HeaderEnv: "OTEL_SERVE_AUTH", ServiceName: "mine"}) {
		t.Fatalf("file holds %+v", got)
	}
	got := readJSON[control.ObservabilitySettings](t, srv.URL, "/observability")
	if !got.Enabled || !got.HeaderEnvSet || got.HeaderEnv != "OTEL_SERVE_AUTH" {
		t.Fatalf("read back %+v", got)
	}
	patch := postProvider(t, srv.URL, "/observability", `{"enabled":false}`)
	patch.Body.Close()
	if got := config.LoadForEdit(config.UserConfigPath()).Observability.OTLP; got.Enabled || got.Endpoint != "http://localhost:4318" {
		t.Fatalf("a partial patch changed more than it named: %+v", got)
	}
}

func TestObservabilityRefusalsCarryTheirOwnCodesAndSaveNothing(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(operatorHandler(s))
	defer srv.Close()
	for _, c := range []struct{ body, code string }{
		{`{"endpoint":"collector:4318"}`, "observability.endpoint_invalid"},
		{`{"endpoint":"https://user:pw@collector"}`, "observability.endpoint_credentials"},
		{`{"headerName":"Host"}`, "observability.header_invalid"},
		{`{"headerEnv":"AWS_SECRET_ACCESS_KEY"}`, "observability.header_env_invalid"},
	} {
		resp := postProvider(t, srv.URL, "/observability", c.body)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), c.code) {
			t.Fatalf("%s = %d %s", c.body, resp.StatusCode, raw)
		}
	}
	if got := config.LoadForEdit(config.UserConfigPath()).Observability.OTLP; got != (config.OTLPConfig{}) {
		t.Fatalf("a refused save wrote %+v", got)
	}
}

func TestANetworkedClientCannotMoveTheTraceExport(t *testing.T) {
	url := signInServer(t, false, "")
	status, raw := postSide(t, url+"/observability", map[string]any{"enabled": true, "endpoint": "https://elsewhere.example"})
	if status != http.StatusForbidden || !strings.Contains(string(raw), "observability.editing_disabled") {
		t.Fatalf("without the grant = %d %s", status, raw)
	}
	if got := config.LoadForEdit(config.UserConfigPath()).Observability.OTLP; got != (config.OTLPConfig{}) {
		t.Fatalf("an ungranted save wrote %+v", got)
	}
}
