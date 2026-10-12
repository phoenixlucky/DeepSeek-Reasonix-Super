package eventwire

import (
	"encoding/json"
	"reasonix/internal/base/secrets"
	"strings"
	"testing"
)

func TestSurfaceArgsRedactionDoesNotChangeNeighbouringTools(t *testing.T) {
	credential := "sk-" + strings.Repeat("x", 48)
	if secrets.RedactCredentials(credential) == credential {
		t.Fatal("fixture is not a recognised credential")
	}
	raw, _ := json.Marshal(map[string]any{"spec_version": 1, "body": []any{map[string]any{"kind": "field", "key": "value", "label": credential, "field": map[string]any{"type": "string", "default": credential}}}})
	got := SurfaceArgs("render_surface", "", string(raw))
	if got == "" || strings.Contains(got, credential) {
		t.Fatalf("redaction: %s", got)
	}
	if got := SurfaceArgs("ask", "", string(raw)); got != string(raw) {
		t.Fatal("ordinary ask changed")
	}
	proxy, _ := json.Marshal(map[string]any{"action": "call", "capability_id": "tool:render_surface", "arguments": json.RawMessage(raw)})
	if got := SurfaceArgs("use_capability", "render_surface", string(proxy)); got == "" || strings.Contains(got, credential) {
		t.Fatal(got)
	}
	if got := SurfaceArgs("use_capability", "", string(proxy)); got == "" || strings.Contains(got, credential) {
		t.Fatal("interrupted/eager arguments were not redacted")
	}
	if got := SurfaceArgs("use_capability", "other_tool", string(proxy)); got != string(proxy) {
		t.Fatal("actual resolved target was ignored")
	}
}
