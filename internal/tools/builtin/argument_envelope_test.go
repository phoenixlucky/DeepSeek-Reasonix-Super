package builtin

import (
	"encoding/json"
	"testing"

	"reasonix/internal/contract/tool"
)

func TestBuiltinArgumentEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		target tool.Tool
		inner  string
	}{
		{readFile{}, `{"path":"lua/mj_main.lua","offset":300,"limit":80}`},
		{bash{}, `{"command":"pwd"}`},
	} {
		t.Run(tc.target.Name(), func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{"arguments": tc.inner})
			if err != nil {
				t.Fatal(err)
			}
			effective, changed := tool.PrepareArguments(tc.target, raw)
			if !changed || string(effective) != tc.inner {
				t.Fatalf("real builtin schema did not normalize input: %s", effective)
			}
		})
	}
}
