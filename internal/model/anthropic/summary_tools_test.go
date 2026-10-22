package anthropic

import (
	"context"
	"reflect"
	"testing"

	"reasonix/internal/contract/provider"
)

func TestSummaryRetainsSchemasWithToolUseDisabled(t *testing.T) {
	c := &client{model: "m", webSearch: true}
	req := provider.Request{Tools: []provider.ToolSchema{{Name: "read_file"}}}
	normal := c.buildRequest(context.Background(), req)
	req.Summary = true
	summary := c.buildRequest(context.Background(), req)
	if !reflect.DeepEqual(normal.Tools, summary.Tools) || summary.ToolChoice == nil || summary.ToolChoice.Type != "none" || normal.ToolChoice != nil {
		t.Fatalf("summary changed the catalog or retained tool use: %+v", summary)
	}
}
