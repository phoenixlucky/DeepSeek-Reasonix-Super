package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/session/control"
)

type callStopper struct {
	control.SessionAPI
	running map[string]bool
}

func (c *callStopper) CancelToolCall(id string) bool {
	was := c.running[id]
	delete(c.running, id)
	return was
}

func TestCancelToolCallStopsARunningCallAndRefusesOneThatEnded(t *testing.T) {
	s := &Server{ctrl: &callStopper{running: map[string]bool{"call 1": true}}}
	mux := http.NewServeMux()
	s.registerToolCallRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tool-calls/call%201/cancel", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first stop: status %d, want 204", rec.Code)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tool-calls/call%201/cancel", nil))
	var reason Reason
	_ = json.Unmarshal(rec.Body.Bytes(), &reason)
	if rec.Code != http.StatusConflict || reason.Code != "tool_call.not_running" {
		t.Fatalf("second stop: status %d code %q, want 409 tool_call.not_running", rec.Code, reason.Code)
	}
}
