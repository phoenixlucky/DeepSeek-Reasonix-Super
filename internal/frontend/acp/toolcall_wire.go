package acp

import "encoding/json"

// toolCall is a "tool_call" update (announces a call, with name/title/kind/rawInput).
type toolCall struct {
	SessionUpdate string             `json:"sessionUpdate"`
	ToolCallID    string             `json:"toolCallId"`
	Name          string             `json:"name,omitempty"`
	Title         string             `json:"title,omitempty"`
	Kind          string             `json:"kind,omitempty"`
	Status        string             `json:"status,omitempty"`
	RawInput      json.RawMessage    `json:"rawInput,omitempty"`
	Locations     []ToolCallLocation `json:"locations,omitempty"`
}

// ToolCallLocation names a file (and optionally a line) a tool call touches, so
// the client can follow along in the editor.
type ToolCallLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

// toolCallUpdateMsg is a "tool_call_update" update (status + result content).
type toolCallUpdateMsg struct {
	SessionUpdate string        `json:"sessionUpdate"`
	ToolCallID    string        `json:"toolCallId"`
	Status        string        `json:"status,omitempty"`
	Content       []toolContent `json:"content,omitempty"`
}

// toolContent wraps a tool result's text, per the ACP tool_call_update shape.
type toolContent struct {
	Type    string       `json:"type"`
	Content ContentBlock `json:"content"`
}
