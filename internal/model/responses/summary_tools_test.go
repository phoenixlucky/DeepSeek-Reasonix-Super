package responses

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"reasonix/internal/contract/provider"
)

func TestSummaryKeepsSchemasWithoutAContinuationOrToolUse(t *testing.T) {
	c := &client{model: "m", mode: "stateful", webSearch: true, lastResponseID: "main-response"}
	prefix := []provider.Message{{Role: provider.RoleUser, Content: "task"}, {Role: provider.RoleAssistant, Content: "finding"}}
	c.expectedPrefixDigest = c.conversationDigest(prefix)
	req := provider.Request{Messages: append(prefix, provider.Message{Role: provider.RoleUser, Content: "summarize"}), Tools: []provider.ToolSchema{{Name: "read_file"}}}
	normal, used, _ := c.buildRequestBody(req)
	if !used {
		t.Fatal("fixture did not exercise the normal continuation")
	}
	req.Summary = true
	summary, used, _ := c.buildRequestBody(req)
	if used || summary["previous_response_id"] != nil || summary["tool_choice"] != "none" || !reflect.DeepEqual(summary["tools"], normal["tools"]) {
		t.Fatalf("summary changed tools or reused a live continuation: %+v", summary)
	}
}

func TestSummaryDoesNotReplaceTheMainResponseContinuation(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprint("invalid=", invalid), func(t *testing.T) {
			var bodies []map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				bodies = append(bodies, body)
				if invalid && len(bodies) == 2 {
					writeEvents(w, `{"type":"response.completed","response":{"id":"poisoned","output":[{"type":"function_call","call_id":"c","name":"echo","arguments":{}}]}}`)
					return
				}
				writeEvents(w, `{"type":"response.output_text.delta","item_id":"msg","delta":"answer"}`,
					fmt.Sprintf(`{"type":"response.completed","response":{"id":"r%d"}}`, len(bodies)))
			}))
			defer srv.Close()
			p := New(Config{Name: "relay", BaseURL: srv.URL, Model: "m"})
			req := provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "one"}}}
			collect(t, p, req)
			req.Messages = append(req.Messages, provider.Message{Role: provider.RoleAssistant, Content: "answer"}, provider.Message{Role: provider.RoleUser, Content: "summarize"})
			req.Summary = true
			chunks := collect(t, p, req)
			if invalid {
				assertTerminalFailure(t, chunks, provider.ErrInvalidFunctionCall)
			}
			req.Summary = false
			req.Messages[len(req.Messages)-1].Content = "two"
			collect(t, p, req)
			if bodies[1]["previous_response_id"] != nil || bodies[2]["previous_response_id"] != "r1" {
				t.Fatalf("summary replaced the main continuation: %+v", bodies)
			}
		})
	}
}
