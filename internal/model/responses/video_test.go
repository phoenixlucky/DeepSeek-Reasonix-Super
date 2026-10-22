package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/contract/provider"
)

func TestAVideoIsReportedNotSilentlyDropped(t *testing.T) {
	c := New(Config{Name: "r", BaseURL: "https://gw.example/v1", Model: "m"}).(*client)
	body, _, _ := c.buildRequestBody(provider.Request{Messages: []provider.Message{
		{Role: provider.RoleUser, Content: "watch"},
		{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "c1", Name: "read_video", Arguments: "{}"}}},
		{Role: provider.RoleTool, ToolCallID: "c1", Name: "read_video", Content: "ok", Videos: []provider.VideoRef{{Name: "clip.mp4", Path: "/x/clip.mp4"}}},
	}})
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), "cannot take video natively") || !strings.Contains(string(raw), "clip.mp4") {
		t.Fatalf("the model is not told the video was not sent: %s", raw)
	}
}
