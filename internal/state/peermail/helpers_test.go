package peermail

import (
	"encoding/json"
	"strconv"
	"testing"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func itoa(i int) string { return strconv.Itoa(i) }
