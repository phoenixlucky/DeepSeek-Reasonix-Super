package agent

import (
	"encoding/json"

	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
)

func (a *Agent) prepareCallArguments(call provider.ToolCall) provider.ToolCall {
	// Recompute from original bytes; persisted metadata never authorizes input.
	call.ArgumentNormalization = nil
	target, _, ambiguous := a.svc.tools.ResolveCall(call.Name)
	if target == nil || len(ambiguous) != 0 {
		return call
	}
	if args, changed := tool.PrepareArguments(target, json.RawMessage(call.Arguments)); changed {
		call.ArgumentNormalization = &provider.ArgumentNormalization{
			Reason: provider.ArgumentEnvelopeUnwrapped, Original: call.Arguments, Effective: string(args),
		}
	}
	return call
}

func (a *Agent) prepareCallBatch(calls []provider.ToolCall) []provider.ToolCall {
	out := append([]provider.ToolCall(nil), calls...)
	for i := range out {
		out[i] = a.prepareCallArguments(out[i])
	}
	return out
}
