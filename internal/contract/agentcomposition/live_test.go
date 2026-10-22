package agentcomposition

import (
	"encoding/json"
	"testing"
)

func TestBindingResourceFormats(t *testing.T) {
	d := Builtins()[0]
	for _, tc := range []struct {
		name  string
		extra string
		valid bool
	}{
		{"legacy", "", true},
		{"live", `,"formatVersion":2,"resourceMode":"live"`, true},
		{"unknown", `,"formatVersion":99,"resourceMode":"live"`, false},
		{"missing_mode", `,"formatVersion":2`, false},
		{"missing_version", `,"resourceMode":"live"`, false},
		{"unknown_mode", `,"formatVersion":2,"resourceMode":"other"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := Binding{Version: Version{Definition: d, Revision: Revision(d)}, Resolved: d, Resources: []Resource{}}
			raw, _ := json.Marshal(b)
			raw = append(raw[:len(raw)-1], []byte(tc.extra+"}")...)
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			b.Seal()
			if err := b.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid = %v, error = %v", tc.valid, err)
			}
		})
	}
}
