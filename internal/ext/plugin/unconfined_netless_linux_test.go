//go:build linux

package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

// A bubblewrap that cannot unshare the network (a container without
// CAP_NET_ADMIN) cannot confine a server whose spec denies network. That server
// starts unwrapped and is recorded, as on a host with no bubblewrap at all.
func TestConfinedServerStartsUnconfinedWhenBwrapCannotUnshareTheNetwork(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do [ \"$a\" = --unshare-net ] && exit 1; done\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "bwrap"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	spec := unconfinedProbeSpec(t, MCPProcessConfined)
	spec.Sandbox.Network = false
	if !launchProbe(t, spec) {
		t.Fatal("a confined server started without a network namespace was not recorded as unconfined")
	}
}
