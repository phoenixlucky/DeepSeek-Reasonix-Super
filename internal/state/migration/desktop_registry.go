package migration

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/state/sessionstore"
)

const desktopRegistryPath = "desktop/workspace-state-v1.json"

// desktopSessionOwners reads which workspace root 1.x Desktop filed each
// session under; the by-id store itself carries no workspace. An unreadable
// registry yields no owners, and those sessions fall back to the chosen workspace.
func desktopSessionOwners(tree fs.FS, home string) map[string]string {
	owners, _, _ := readDesktopRegistry(tree, home)
	return owners
}

// desktopArchivedSessions reads which conversations 1.x Desktop had filed away.
// The archive list lived in the shell rather than the session store, so without it
// every archived conversation comes back as an active one after the move.
func desktopArchivedSessions(tree fs.FS, home string) map[string]bool {
	_, archived, _ := readDesktopRegistry(tree, home)
	return archived
}

// readDesktopRegistry reads both halves of the 1.x registry in one pass: which
// workspace owns each conversation, and which ones the person had archived.
func readDesktopRegistry(tree fs.FS, home string) (map[string]string, map[string]bool, error) {
	raw, err := fs.ReadFile(tree, path.Join(home, desktopRegistryPath))
	if err != nil {
		return nil, nil, err
	}
	var state struct {
		Workspaces map[string]struct {
			Root       string   `json:"root"`
			SessionIDs []string `json:"sessionIds"`
		} `json:"workspaces"`
		ArchivedSessionIDs []string `json:"archivedSessionIds"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, nil, err
	}
	owners := map[string]string{}
	for _, ws := range state.Workspaces {
		for _, id := range ws.SessionIDs {
			owners[id] = ws.Root
		}
	}
	archived := map[string]bool{}
	for _, id := range state.ArchivedSessionIDs {
		if id = strings.TrimSpace(id); id != "" {
			archived[id] = true
		}
	}
	return owners, archived, nil
}

// routeV4 places each conversation of a 1.x Desktop store: by the registry's workspace
// root, else by the working folder its own header records. A root that is known but
// gone is orphaned and falls back to the chosen folder; none at all is not placed.
// The store always gets a route, so an unreadable registry cannot make it unscoped.
func routeV4(owners map[string]string, note func(root string)) func(sessionstore.V4Origin) sessionstore.V4Placement {
	return func(o sessionstore.V4Origin) sessionstore.V4Placement {
		root := strings.TrimSpace(owners[o.ID])
		if root == "" {
			root = o.Cwd
		}
		if root == "" {
			return sessionstore.V4Placement{}
		}
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			return sessionstore.V4Placement{Known: true}
		}
		note(root)
		return sessionstore.V4Placement{Dir: config.ProjectSessionDir(root), Known: true}
	}
}
