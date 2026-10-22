package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/platform/openwith"
)

const (
	codeOpenWithUnknown     = "openwith.unknown"
	codeOpenWithUnavailable = "openwith.unavailable"
	codeOpenWithFailed      = "openwith.failed"
)

// Seams, so the contract can be tested without starting a program.
var (
	openWithList = func(forbidden []string) []openwith.Status { return openwith.List(os.Getenv, forbidden) }
	openWithOpen = func(id, dir string, forbidden []string) (string, error) {
		return openwith.Open(os.Getenv, id, dir, forbidden)
	}
)

func (h *Hub) registerOpenWithRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /host/open-with", h.openWithTargets)
	mux.HandleFunc("POST /host/open-with", h.openWithProject)
}

// openWithTargets lists the programs this machine can open a project in. Which
// programs exist is the kernel's table; the page only draws what it is told.
func (h *Hub) openWithTargets(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"targets": openWithList(h.openWithRoots())})
}

func (h *Hub) openWithRoots() []string {
	refs := h.sessionRoots()
	roots := make([]string, 0, len(refs))
	for _, ref := range refs {
		roots = append(roots, ref.dir)
	}
	return roots
}

// openWithProject starts one declared program on a project the window lists.
// The page names a target id and a listed root, never a program or a path to
// run: the table owns the executable, and the folder is the window's own copy.
func (h *Hub) openWithProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Root string `json:"root"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		badBody(w)
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		missingField(w, "id")
		return
	}
	listed, ok := h.listedWorkspace(strings.TrimSpace(req.Root))
	if !ok {
		refuse(w, http.StatusNotFound, codeWorkspaceUnknown, "that folder is not a project in this window", nil)
		return
	}
	abs, err := filepath.Abs(listed)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		refuse(w, http.StatusNotFound, codeWorkspaceMissing, "the project folder is not on disk", nil)
		return
	}
	name, err := openWithOpen(req.ID, abs, h.openWithRoots())
	switch {
	case errors.Is(err, openwith.ErrUnknown):
		refuse(w, http.StatusNotFound, codeOpenWithUnknown, "no such program in the list", nil)
	case errors.Is(err, openwith.ErrUnavailable):
		refuse(w, http.StatusNotFound, codeOpenWithUnavailable, "that program is not installed on this machine", nil)
	case err != nil:
		saveFailed(w, http.StatusInternalServerError, codeOpenWithFailed, err)
	default:
		writeJSON(w, map[string]any{"target": name})
	}
}
