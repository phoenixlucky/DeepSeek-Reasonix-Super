package eventwire

import (
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/contract/delivery"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

func TestToolWirePreservesDeliveriesAndNormalizedArguments(t *testing.T) {
	n := &provider.ArgumentNormalization{
		Reason: provider.ArgumentEnvelopeUnwrapped, Original: `{"arguments":{"path":"report.txt"}}`, Effective: `{"path":"report.txt"}`,
	}
	files := []delivery.File{{Path: "report.txt", Description: "Report"}}
	wire := ToWire(event.Event{Kind: event.ToolResult, Tool: event.Tool{
		Name: "present_files", Args: n.Effective, ArgumentNormalization: n, Deliveries: files,
	}})
	if wire.Tool == nil || len(wire.Tool.Deliveries) != 1 || wire.Tool.Deliveries[0] != files[0] ||
		wire.Tool.ArgumentNormalization == nil || wire.Tool.ArgumentNormalization.Original != n.Original ||
		wire.Tool.ArgumentNormalization.Effective != n.Effective {
		t.Fatalf("tool wire lost a field: %+v", wire.Tool)
	}
}

func TestArgumentNormalizationPreservesSurfaceRedaction(t *testing.T) {
	credential := "sk-" + strings.Repeat("x", 48)
	effective, _ := json.Marshal(map[string]any{
		"spec_version": 1,
		"body":         []any{map[string]any{"kind": "field", "key": "value", "label": credential, "field": map[string]any{"type": "string"}}},
	})
	original, _ := json.Marshal(map[string]json.RawMessage{"arguments": effective})
	for _, proxy := range []bool{false, true} {
		name := "render_surface"
		raw, normalized := original, effective
		if proxy {
			name = "use_capability"
			raw, _ = json.Marshal(map[string]any{"action": "call", "capability_id": "tool:render_surface", "arguments": json.RawMessage(original)})
			normalized, _ = json.Marshal(map[string]any{"action": "call", "capability_id": "tool:render_surface", "arguments": json.RawMessage(effective)})
		}
		n := &provider.ArgumentNormalization{Reason: provider.ArgumentEnvelopeUnwrapped, Original: string(raw), Effective: string(normalized)}
		wire := ToWire(event.Event{Kind: event.ToolDispatch, Tool: event.Tool{
			Name: name, Args: string(normalized), ArgumentNormalization: n,
		}})
		encoded, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), credential) {
			t.Fatalf("proxy=%v: normalization bypassed surface redaction", proxy)
		}
		if !strings.Contains(n.Original, credential) || !strings.Contains(n.Effective, credential) {
			t.Fatal("wire projection changed canonical normalization metadata")
		}
	}
}
