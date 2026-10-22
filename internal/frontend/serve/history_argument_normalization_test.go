package serve

import (
	"reflect"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/eventwire"
	"reasonix/internal/contract/provider"
)

func TestArgumentNormalizationLiveAndHistoryMatch(t *testing.T) {
	n := &provider.ArgumentNormalization{
		Reason:   "redundant_arguments_envelope",
		Original: `{"arguments":{"path":"a"}}`, Effective: `{"path":"a"}`,
	}
	wire := eventwire.ToWire(event.Event{Kind: event.ToolResult, Tool: event.Tool{
		ID: "1", Name: "read_file", Args: n.Effective, ArgumentNormalization: n,
	}})
	history := historyMessages([]provider.Message{{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{
		ID: "1", Name: "read_file", Arguments: n.Original, ArgumentNormalization: n,
	}}}})
	call := history[0].ToolCalls[0]
	if wire.Tool.Args != call.Arguments || !reflect.DeepEqual(wire.Tool.ArgumentNormalization, call.ArgumentNormalization) {
		t.Fatalf("live and restored arguments differ: live=%+v history=%+v", wire.Tool, call)
	}
}
