package serve

import (
	"encoding/json"
	"net/http"
	"testing"

	"reasonix/internal/contract/config"
)

const editSwitchesBase = `"name":"relay","models":["alpha"],"default":"alpha"`

func postEdit(t *testing.T, base, extra string) (int, string) {
	t.Helper()
	resp := postProvider(t, base, "/providers/edit", `{`+editSwitchesBase+extra+`}`)
	defer resp.Body.Close()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Code
}

func relayEntry(t *testing.T) *config.ProviderEntry {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.Provider("relay")
	if !ok {
		t.Fatal("the provider disappeared")
	}
	return entry
}

// The panel holds both declarations in the same draft as the rest of the
// form, so one save has to land them together.
func TestEditSavesTheTwoSwitchesTogether(t *testing.T) {
	srv := newContinuationServerRunning(t, "plain/beta")
	if code, got := postEdit(t, srv.URL, `,"webSearch":true,"continuation":"stateless"`); code != http.StatusNoContent {
		t.Fatalf("edit = %d %q, want 204", code, got)
	}
	entry := relayEntry(t)
	if entry.WebSearch == nil || !*entry.WebSearch || entry.ResponsesMode != "stateless" {
		t.Fatalf("saved entry = webSearch %v, mode %q", entry.WebSearch, entry.ResponsesMode)
	}
}

// A form that does not show a switch must not reset it: unsent is a different
// answer from sent off.
func TestEditWithoutTheSwitchesLeavesThemAlone(t *testing.T) {
	srv := newContinuationServerRunning(t, "plain/beta")
	if code, got := postEdit(t, srv.URL, `,"webSearch":true,"continuation":"stateless"`); code != http.StatusNoContent {
		t.Fatalf("seed edit = %d %q", code, got)
	}
	if code, _ := postEdit(t, srv.URL, ``); code != http.StatusNoContent {
		t.Fatalf("plain edit = %d", code)
	}
	entry := relayEntry(t)
	if entry.WebSearch == nil || !*entry.WebSearch || entry.ResponsesMode != "stateless" {
		t.Fatalf("an unrelated save changed the switches: %+v", entry)
	}
}

// One bad switch refuses the whole save: nothing else in the draft is written
// either, so the file never holds half of what the panel showed.
func TestEditRefusingASwitchSavesNothing(t *testing.T) {
	srv := newContinuationServerRunning(t, "plain/beta")
	for _, tc := range []struct{ name, extra, code string }{
		{"unknown mode", `,"webSearch":true,"continuation":"websocket-v2"`, "request.bad_value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, got := postEdit(t, srv.URL, tc.extra)
			if code != http.StatusBadRequest || got != tc.code {
				t.Fatalf("edit = %d %q, want 400 %q", code, got, tc.code)
			}
			if entry := relayEntry(t); entry.WebSearch != nil || entry.ResponsesMode != "" {
				t.Fatalf("a refused edit wrote anyway: %+v", entry)
			}
		})
	}
	resp := postProvider(t, srv.URL, "/providers/edit", `{"name":"plain","models":["beta"],"default":"beta","webSearch":true,"continuation":"stateless"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("switches a chat-completions wire has no field for = %d, want 400", resp.StatusCode)
	}
	cfg, _ := config.Load()
	if p, _ := cfg.Provider("plain"); p.WebSearch != nil || p.ResponsesMode != "" {
		t.Fatalf("a refused edit wrote anyway: %+v", p)
	}
}
