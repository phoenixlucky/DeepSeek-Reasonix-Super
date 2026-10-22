package anthropic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/contract/provider"
)

// This wire has no video block, so a read that reaches it is said to the model
// in words rather than dropped without a trace.
func TestAVideoIsReportedNotSilentlyDropped(t *testing.T) {
	c := &client{model: "claude-opus-4-8", vision: true}
	req := c.buildRequest(context.Background(), provider.Request{Messages: []provider.Message{
		{Role: provider.RoleUser, Content: "watch"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "c1", Name: "read_video", Arguments: "{}"}}},
		{Role: provider.RoleTool, ToolCallID: "c1", Name: "read_video", Content: "ok", Videos: []provider.VideoRef{{Name: "clip.mp4", Path: "/x/clip.mp4"}}},
	}})
	body, _ := json.Marshal(req)
	if !strings.Contains(string(body), "cannot take video natively") || !strings.Contains(string(body), "clip.mp4") {
		t.Fatalf("the model is not told the video was not sent: %s", body)
	}
}
