package provider

import "errors"

// ErrSummaryToolsUnsupported refuses retained schemas before an unsafe broker POST.
var ErrSummaryToolsUnsupported = errors.New("provider cannot guarantee disabled summary tools")

// Request is a single completion request.
type Request struct {
	Messages    []Message
	Tools       []ToolSchema
	Temperature *float64 // nil = omit; non-nil = send the value, including 0
	MaxTokens   int
	// ResponseFormat, when non-nil, asks the endpoint for structured JSON
	// output (Responses: text.format.type=json_object). Nil omits the field
	// entirely — the common path must stay byte-stable for prompt caching.
	ResponseFormat *ResponseFormat `json:"ResponseFormat,omitempty"`
	EffortOverride string          `json:"EffortOverride,omitempty"` // per-call reasoning-depth override; adapters apply it only when the endpoint's effort vocabulary accepts it
	// Mode is the session's model mode id. An adapter sends only a mode its
	// config declared for this endpoint, so an unknown one reaches no wire.
	Mode string `json:"Mode,omitempty"`
	// Summary asks for a compaction briefing with tool use disabled. It crosses
	// the provider broker so remote and local summaries have the same contract.
	Summary bool `json:"summary,omitempty"`
}

// ResponseFormat asks a provider to constrain its output shape.
type ResponseFormat struct {
	// Type is the structured format: "json_object" is the only shape the
	// Responses endpoints currently define (MiMo/DashScope/OpenAI).
	Type string `json:"type"`
}
