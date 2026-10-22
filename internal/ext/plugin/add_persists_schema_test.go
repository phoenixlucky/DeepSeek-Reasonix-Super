package plugin

import (
	"context"
	"testing"
	"time"
)

// A server connected through Add (an explicit connect, as the settings page
// and /mcp add do) must leave its handshake in the cache like one the host
// started itself: the next session registers it from the cache, and a session
// that closes right after connecting must not lose it.
func TestAddLeavesTheHandshakeInTheCacheBeforeCloseReturns(t *testing.T) {
	redirectCache(t)
	spec := helperSpec()

	host := NewHost()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := host.Add(ctx, spec); err != nil {
		t.Fatal(err)
	}
	host.Close()

	cs, ok := LoadCachedSchemaForSpec(spec)
	if !ok {
		t.Fatal("a connected server left no cache behind")
	}
	if len(cs.Tools) != 2 {
		t.Fatalf("cached %d tools, want the server's 2", len(cs.Tools))
	}
}
