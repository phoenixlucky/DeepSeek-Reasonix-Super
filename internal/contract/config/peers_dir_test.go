package config

import (
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
)

// The peer-mail spool holds other sessions' message bodies. It lives under the
// state root, and it is denied to every reader the way the remote launch tokens
// are: one composition feeds both the OS sandbox and the in-process tools.
func TestPeersDirIsUnderTheStateRootAndDeniedToReaders(t *testing.T) {
	home := testenv.TempDir(t)
	cfg := Default()
	cfg.roots = RootsForHome(home)

	dir := cfg.Roots().PeersDir()
	if dir == "" || filepath.Base(dir) != "peers" || filepath.Dir(dir) != filepath.Dir(cfg.Roots().SessionDir()) {
		t.Fatalf("PeersDir = %q, want <state root>/peers beside sessions", dir)
	}
	for _, root := range cfg.ForbiddenRootsForRoot(testenv.TempDir(t)) {
		if samePath(root, dir) {
			return
		}
	}
	t.Fatalf("ForbiddenRootsForRoot does not list %s", dir)
}
