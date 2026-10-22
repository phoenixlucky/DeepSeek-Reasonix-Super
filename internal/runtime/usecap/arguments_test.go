package usecap

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/runtime/capability"
)

type envelopeTarget struct{ name string }

func (t envelopeTarget) Name() string      { return t.name }
func (envelopeTarget) Description() string { return "" }
func (envelopeTarget) ReadOnly() bool      { return true }
func (envelopeTarget) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
}
func (envelopeTarget) Execute(context.Context, json.RawMessage) (string, error) {
	panic("preparation must not execute tools")
}

func TestPrepareProxyArgumentsUsesRegisteredTargetSchemas(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(envelopeTarget{"read_file"})
	reg.Add(envelopeTarget{plugin.ModelToolName("docs", "read")})
	proxy := NewUseCapabilityTool(context.Background(), nil, nil, reg, capability.NewLedger(), &capability.Audit{}, func() capability.Catalog { return capability.Catalog{} })
	for _, id := range []string{"tool:read_file", "mcp-tool:docs/read"} {
		raw := json.RawMessage(`{"action":"call","capability_id":"` + id + `","arguments":{"arguments":{"path":"a"}}}`)
		got, changed := tool.PrepareArguments(proxy, raw)
		if !changed || string(proxy.TargetArgs(got)) != `{"path":"a"}` {
			t.Fatalf("%s: target envelope not normalized: %s", id, got)
		}
	}
	for _, raw := range []string{
		`{"action":"call","capability_id":"tool:read_file","arguments":{"path":"a"}}`,
		`{"action":"call","capability_id":"mcp-tool:missing/read","arguments":{"arguments":{"path":"a"}}}`,
		`{"action":"call","capability_id":"tool:read_file","arguments":{"arguments":{"path":"a"}},"action":"search"}`,
		`{"arguments":{"action":"call","capability_id":"tool:read_file","arguments":{"path":"a"}}}`,
	} {
		got, changed := tool.PrepareArguments(proxy, json.RawMessage(raw))
		if changed || string(got) != raw {
			t.Fatalf("legal, unknown, or ambiguous proxy call changed: %s", got)
		}
	}
}
