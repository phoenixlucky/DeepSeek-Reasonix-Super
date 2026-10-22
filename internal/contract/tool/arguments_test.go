package tool

import (
	"encoding/json"
	"testing"
)

func TestUnwrapArguments(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"limit":{"type":"integer","minimum":1}},"required":["path"],"additionalProperties":false}`)
	for _, tc := range []struct {
		name, input, want string
	}{
		{"object", `{"arguments":{"path":"a","limit":80}}`, `{"path":"a","limit":80}`},
		{"string", `{"arguments":"{\"path\":\"a\",\"limit\":80}"}`, `{"path":"a","limit":80}`},
		{"valid", `{"path":"a"}`, ""},
		{"mixed", `{"path":"a","arguments":{"path":"b"}}`, ""},
		{"extra outer", `{"arguments":{"path":"a"},"timeout":1}`, ""},
		{"nested", `{"arguments":{"arguments":{"path":"a"}}}`, ""},
		{"boolean", `{"arguments":true}`, ""},
		{"null", `{"arguments":null}`, ""},
		{"array", `{"arguments":[{"path":"a"}]}`, ""},
		{"top string", `"{\"path\":\"a\"}"`, ""},
		{"missing", `{"arguments":{"limit":1}}`, ""},
		{"type", `{"arguments":{"path":false}}`, ""},
		{"range", `{"arguments":{"path":"a","limit":0}}`, ""},
		{"fraction", `{"arguments":{"path":"a","limit":1.5}}`, ""},
		{"unknown", `{"arguments":{"path":"a","other":1}}`, ""},
		{"duplicate outer", `{"arguments":{"path":"a"},"arguments":{"path":"b"}}`, ""},
		{"duplicate inner", `{"arguments":{"path":"a","path":"b"}}`, ""},
		{"duplicate escaped", `{"arguments":"{\"path\":\"a\",\"\\u0070ath\":\"b\"}"}`, ""},
		{"truncated", `{"arguments":"{\"path\":\"a\""}`, ""},
		{"trailing", `{"arguments":"{\"path\":\"a\"} {}"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := UnwrapArguments(schema, json.RawMessage(tc.input))
			want := tc.want
			if want == "" {
				want = tc.input
			}
			if string(got) != want || changed != (tc.want != "") {
				t.Fatalf("got %s, changed=%v; want %s", got, changed, want)
			}
		})
	}
}

func TestUnwrapArgumentsRequiresUnambiguousSchema(t *testing.T) {
	for _, schema := range []string{
		`{}`,
		`{"type":"object"}`,
		`{"type":"object","properties":{"path":{"type":"string"}}}`,
		`{"type":"object","properties":{"arguments":{"type":"object"},"path":{"type":"string"}},"required":["path"]}`,
		`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"patternProperties":{".*":{}}}`,
		`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"anyOf":[{},{}]}`,
		`{"type":"object","properties":{"path":{"$ref":"file:///never-read.json"}},"required":["path"]}`,
	} {
		input := json.RawMessage(`{"arguments":{"path":"a"}}`)
		got, changed := UnwrapArguments(json.RawMessage(schema), input)
		if changed || string(got) != string(input) {
			t.Fatalf("schema %s unexpectedly unwrapped %s", schema, got)
		}
	}
}

func TestUnwrapArgumentsPreservesNumbersAndStringBytes(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"},"text":{"type":"string"}},"required":["id","text"]}`)
	inner := `{"id":9007199254740993,"text":"echo $HOME\n中文\\path"}`
	wrapped, err := json.Marshal(map[string]string{"arguments": inner})
	if err != nil {
		t.Fatal(err)
	}
	got, changed := UnwrapArguments(schema, wrapped)
	if !changed || string(got) != inner {
		t.Fatalf("inner bytes changed: %s", got)
	}
}
