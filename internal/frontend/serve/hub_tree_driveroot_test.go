package serve

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
)

func spellings(root string) map[string]string {
	out := map[string]string{
		"trailing separator": root + string(filepath.Separator),
		"redundant dot":      filepath.Join(root, ".") + string(filepath.Separator) + ".",
	}
	if runtime.GOOS == "windows" {
		out["forward slashes"] = filepath.ToSlash(root)
		out["upper case"] = strings.ToUpper(root)
		out["lower case"] = strings.ToLower(root)
	}
	return out
}

// A pane belongs to a folder however that folder is spelled: the list of
// remembered folders and the pane's own root are written by different code.
func TestDrivesRootIgnoresHowTheFolderIsSpelled(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	root := testenv.TempDir(t)
	h := NewHub(HubOptions{})
	rt := hubRuntime(t, h, root)
	if !drivesRoot(rt, rt.Server.Controller().WorkspaceRoot()) {
		t.Fatal("a pane does not drive its own root")
	}
	for name, spelled := range spellings(rt.Server.Controller().WorkspaceRoot()) {
		if !drivesRoot(rt, spelled) {
			t.Errorf("%s: %q is not seen as the pane's folder %q", name, spelled, rt.Server.Controller().WorkspaceRoot())
		}
	}
	if drivesRoot(rt, testenv.TempDir(t)) {
		t.Error("another folder was taken for the pane's")
	}
}
