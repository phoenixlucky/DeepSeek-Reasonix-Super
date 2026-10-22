package openai

type chatRequest struct {
	Model               string         `json:"model"`
	Messages            []chatMessage  `json:"messages"`
	Tools               []chatTool     `json:"tools,omitempty"`
	ToolChoice          string         `json:"tool_choice,omitempty"`
	Stream              bool           `json:"stream"`
	StreamOptions       *streamOptions `json:"stream_options,omitempty"`
	Temperature         *float64       `json:"temperature,omitempty"`
	MaxTokens           int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string         `json:"reasoning_effort,omitempty"`
	Thinking            *thinkingMode  `json:"thinking,omitempty"`
	ExtraBody           map[string]any `json:"-"`
}

func (r chatRequest) forSummary(summary bool) chatRequest {
	if summary && len(r.Tools) > 0 {
		r.ToolChoice = "none"
		r.ExtraBody = omitExtraBodyFields(r.ExtraBody, "tool_choice")
	}
	return r
}
