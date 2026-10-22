package tool

import "testing"

func TestNestedCallIDNamesItsCaller(t *testing.T) {
	id := NestedCallID("call_7", 3)
	if !IsNestedCallOf(id, "call_7") {
		t.Fatalf("%q is not recognised as a call made by call_7", id)
	}
	for _, other := range []struct{ id, parent string }{
		{"call_7", "call_7"},
		{"call_70.1", "call_7"},
		{"call_7.x", "call_7"},
		{"call_7.0", "call_7"},
		{"call_7.01", "call_7"},
		{"call_7.1.2", "call_7"},
		{".1", ""},
	} {
		if IsNestedCallOf(other.id, other.parent) {
			t.Errorf("IsNestedCallOf(%q, %q) = true", other.id, other.parent)
		}
	}
}
