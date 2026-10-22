package openai

import (
	"encoding/json"
	"testing"
)

// Compatible servers disagree about the type of `code` in an error event: OpenAI
// and DeepSeek send a string, gateways in front of them send a number. A decoder
// that insists on one of them fails the whole message, and the person sees a
// parse error instead of the reason the request was refused.
func TestStreamErrorAcceptsAStringOrNumericCode(t *testing.T) {
	cases := []struct {
		name string
		body string
		code string
		msg  string
	}{
		{
			name: "string code, as OpenAI and DeepSeek send it",
			body: `{"error":{"message":"Invalid request","type":"invalid_request_error","code":"invalid_request"}}`,
			code: "invalid_request",
			msg:  "Invalid request",
		},
		{
			name: "numeric code, as a gateway sent it",
			body: `{"error":{"message":"Invalid request: check the parameters and request format.","type":"invalid_request_error","code":400}}`,
			code: "400",
			msg:  "Invalid request: check the parameters and request format.",
		},
		{
			name: "no code at all",
			body: `{"error":{"message":"Invalid request","type":"invalid_request_error"}}`,
			code: "",
			msg:  "Invalid request",
		},
		{
			name: "null code",
			body: `{"error":{"message":"Invalid request","code":null}}`,
			code: "",
			msg:  "Invalid request",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp streamResponse
			if err := json.Unmarshal([]byte(tc.body), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Error == nil {
				t.Fatal("the error event was dropped")
			}
			if got := resp.Error.Code.String(); got != tc.code {
				t.Fatalf("code = %q, want %q", got, tc.code)
			}
			if resp.Error.Message != tc.msg {
				t.Fatalf("message = %q, want %q", resp.Error.Message, tc.msg)
			}
		})
	}
}
