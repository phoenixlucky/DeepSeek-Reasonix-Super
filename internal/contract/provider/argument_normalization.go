package provider

const ArgumentEnvelopeUnwrapped = "redundant_arguments_envelope"

// ArgumentNormalization is host-only evidence of a redundant envelope removal.
// Arguments remains the original provider output, including signed payloads.
type ArgumentNormalization struct {
	Reason    string `json:"reason"`
	Original  string `json:"original"`
	Effective string `json:"effective"`
}

// ExecutionArguments reads the host's effective input without rewriting history.
func (c ToolCall) ExecutionArguments() string {
	if c.ArgumentNormalization != nil && c.ArgumentNormalization.Reason == ArgumentEnvelopeUnwrapped &&
		c.ArgumentNormalization.Original == c.Arguments {
		return c.ArgumentNormalization.Effective
	}
	return c.Arguments
}

func hasArgumentNormalization(calls []ToolCall) bool {
	for _, call := range calls {
		if call.ArgumentNormalization != nil {
			return true
		}
	}
	return false
}

func withoutArgumentNormalization(calls []ToolCall) []ToolCall {
	if !hasArgumentNormalization(calls) {
		return calls
	}
	out := append([]ToolCall(nil), calls...)
	for i := range out {
		out[i].ArgumentNormalization = nil
	}
	return out
}
