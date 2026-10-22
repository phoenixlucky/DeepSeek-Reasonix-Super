package sessionstore

import (
	"os"
	"strings"
	"testing"
)

// 1.x Desktop names a conversation it created itself desktop-<hex> or
// desktop-manual-<hex>; both import like the bare hex ids.
func TestImportV4AcceptsADesktopPrefixedSessionID(t *testing.T) {
	for _, id := range []string{
		"desktop-3984e411afde05959f793453a1ee1cdc",
		"desktop-manual-204ce9bca4ff7fb3ae03f51c4eb91ee5",
	} {
		s, sessions := v4StoreWith(t, id)
		n, skipped, err := ImportV4From(os.DirFS(s.Root), sessions, nil, nil)
		if err != nil || n != 1 || len(skipped) != 0 {
			t.Fatalf("%s: imported %d, skipped %+v, err %v; want the session imported", id, n, skipped, err)
		}
		if _, err := os.Stat(v4ImportPath(sessions, id)); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
}

func TestIsV4SessionIDKeepsToTheKnownShapes(t *testing.T) {
	hex32 := strings.Repeat("ab", 16)
	for id, want := range map[string]bool{
		hex32:                                true,
		"desktop-" + hex32:                   true,
		"desktop-manual-" + hex32:            true,
		"desktop-manual-desktop-" + hex32:    false,
		"desktop-" + strings.ToUpper(hex32):  false,
		"desktop-" + hex32[:8]:               false,
		"desktop-":                           false,
		"manual-" + hex32:                    false,
		"desktop-../" + hex32:                false,
		"desktop-" + hex32 + "/x":            false,
		"desktop-" + strings.Repeat("a", 66): false,
		"not-a-hex-id":                       false,
	} {
		if got := isV4SessionID(id); got != want {
			t.Errorf("isV4SessionID(%q) = %v, want %v", id, got, want)
		}
	}
}
