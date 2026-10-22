package agentcomposition

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDefinitionAuthorityBySourceAndModule(t *testing.T) {
	cases := []struct {
		module, raw string
		projectOK   bool
	}{
		{"mcp", `{}`, true},
		{"mcp", `{"callTimeoutSeconds":5,"startupTimeoutSeconds":10}`, true},
		{"mcp", `{"args":["other-package"]}`, false},
		{"mcp", `{"args":[]}`, false},
		{"mcp", `{"url":"https://other.invalid/mcp"}`, false},
		{"mcp", `{"env":{"PATH":"${OTHER_PATH}"}}`, false},
		{"mcp", `{"env":{}}`, false},
		{"mcp", `{"headers":{"Authorization":"${OTHER_TOKEN}"}}`, false},
		{"mcp", `{"headers":{}}`, false},
		{"extensions", `{"timeoutMillis":5000}`, true},
		{"extensions", `{"args":["other-script"]}`, false},
		{"extensions", `{"env":{"NODE_OPTIONS":"${OTHER_OPTIONS}"}}`, false},
	}
	for _, scope := range []string{"builtin", "user", "project"} {
		for _, tc := range cases {
			t.Run(scope+"/"+tc.module+"/"+tc.raw, func(t *testing.T) {
				d := Builtins()[0]
				d.ID = scope + ":test"
				selection := Selection{Mode: Select, Members: []Module{{ID: "authorized", Parameters: json.RawMessage(tc.raw)}}}
				if tc.module == "mcp" {
					d.MCP = selection
				} else {
					d.Extensions = selection
				}
				err := d.ValidateForExecution()
				data, marshalErr := json.Marshal(d)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				_, decodeErr := Decode(data)
				if scope == "project" && !tc.projectOK {
					if !errors.Is(err, ErrInvalid) || !errors.Is(decodeErr, ErrInvalid) {
						t.Fatalf("repository changed a launch: validate=%v decode=%v", err, decodeErr)
					}
				} else if err != nil || decodeErr != nil {
					t.Fatalf("validate=%v decode=%v", err, decodeErr)
				}
			})
		}
	}
}
