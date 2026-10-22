package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/session/control"
)

type echoSide struct{ prompts []string }

func (e *echoSide) Ask(_ context.Context, prompt string, _ event.Sink) (string, error) {
	e.prompts = append(e.prompts, prompt)
	return "echo: " + prompt, nil
}

func sideServer(t *testing.T, factory control.SideFactory) *httptest.Server {
	t.Helper()
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, Label: "side", ModelRef: config.UnconfiguredRef, SessionDir: testenv.TempDir(t), SideFactory: factory})
	t.Cleanup(ctrl.Close)
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)
	return srv
}

func TestSideRoutesOpenAskAndClose(t *testing.T) {
	conv := &echoSide{}
	srv := sideServer(t, func() (control.SideConversation, error) { return conv, nil })

	status, raw := postSide(t, srv.URL+"/side", map[string]any{})
	var opened struct {
		ID string `json:"id"`
	}
	if status != http.StatusOK || json.Unmarshal(raw, &opened) != nil || opened.ID == "" {
		t.Fatalf("open = %d %s", status, raw)
	}
	status, raw = postSide(t, srv.URL+"/side/"+opened.ID+"/ask", map[string]any{"question": "why?", "quotes": []string{"the passage"}})
	if status != http.StatusOK || !strings.Contains(string(raw), "echo: Quoted from the main conversation (passage 1):") || !strings.Contains(string(raw), "why?") {
		t.Fatalf("ask = %d %s", status, raw)
	}

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/side/"+opened.ID, nil)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusNoContent {
		t.Fatalf("close = %v %v", res, err)
	}
	status, raw = postSide(t, srv.URL+"/side/"+opened.ID+"/ask", map[string]any{"question": "again"})
	if status != http.StatusNotFound || !strings.Contains(string(raw), codeSideUnknown) {
		t.Fatalf("a closed one = %d %s", status, raw)
	}
}

func TestSideRoutesRefuseWithTheirOwnCodes(t *testing.T) {
	none := sideServer(t, nil)
	if status, raw := postSide(t, none.URL+"/side", map[string]any{}); status != http.StatusNotFound || !strings.Contains(string(raw), codeSideUnavailable) {
		t.Fatalf("no runner = %d %s", status, raw)
	}
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	cfg := config.LoadForEdit(config.UserConfigPath())
	cfg.Agent.MaxSideSessions = 3
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	srv := sideServer(t, func() (control.SideConversation, error) { return &echoSide{}, nil })
	_, raw := postSide(t, srv.URL+"/side", map[string]any{})
	var opened struct{ ID string }
	_ = json.Unmarshal(raw, &opened)
	if status, raw := postSide(t, srv.URL+"/side/"+opened.ID+"/ask", map[string]any{"question": "  "}); status != http.StatusBadRequest || !strings.Contains(string(raw), codeSideEmpty) {
		t.Fatalf("empty = %d %s", status, raw)
	}
	for range 2 {
		postSide(t, srv.URL+"/side", map[string]any{})
	}
	if status, raw := postSide(t, srv.URL+"/side", map[string]any{}); status != http.StatusConflict || !strings.Contains(string(raw), codeSideLimit) {
		t.Fatalf("limit = %d %s", status, raw)
	}
}

func postSide(t *testing.T, url string, body any) (int, []byte) {
	t.Helper()
	raw, _ := json.Marshal(body)
	res, err := http.Post(url, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return res.StatusCode, []byte(out.String())
}
