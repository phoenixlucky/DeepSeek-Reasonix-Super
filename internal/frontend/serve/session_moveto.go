package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/state/sessionstore"
)

// moveSessionTo re-homes one conversation into another listed project. What it
// carries is the conversation and the person's own labels; the target project
// decides what it may do from then on. A conversation a window has open, or any
// writer holds, is refused rather than pulled out from under its work.
func (h *Hub) moveSessionTo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string `json:"path"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badBody(w)
		return
	}
	if strings.TrimSpace(body.Target) == "" {
		missingField(w, "target")
		return
	}
	abs, err := filepath.Abs(strings.TrimSpace(body.Path))
	if err != nil {
		refuse(w, http.StatusBadRequest, codeSessionBadPath, "the session path could not be resolved", nil)
		return
	}
	path, ok := h.listedSessionPaths()[sessionstore.CanonicalSessionPath(abs)]
	if !ok {
		refuse(w, http.StatusForbidden, "session.outside_workspace", "path outside a known workspace", nil)
		return
	}
	known := h.knownWorkspaceDir(strings.TrimSpace(body.Target))
	if known == "" {
		refuse(w, http.StatusForbidden, "workspace.unknown", "select a known workspace to move the conversation to", nil)
		return
	}
	if h.openSessions()[sessionstore.CanonicalSessionPath(path)] != "" {
		busy(w, "session.move_open", "this conversation is open in a window; close it first", nil)
		return
	}
	moved, err := sessionstore.MoveSession(path, SessionDirFor(known), known)
	var held *sessionstore.SessionLeaseError
	switch {
	case errors.Is(err, agentcomposition.ErrWorkspaceBound):
		busy(w, "agent_composition.workspace_bound", "this conversation's composition is bound to its project; start a new conversation in the target project", nil)
	case errors.Is(err, sessionstore.ErrSameFolder):
		busy(w, "session.same_project", "the conversation is already in that project", nil)
	case errors.As(err, &held):
		if who := sessionHolder(held); who != nil {
			busy(w, "session.move_held_by", "another process holds this conversation open", who)
			return
		}
		busy(w, "session.move_held", "this conversation is still being written to", nil)
	case errors.Is(err, sessionstore.ErrSessionDisplayReadModelDamaged):
		busy(w, "session.unreadable", "this conversation's record is incomplete", nil)
	case errors.Is(err, sessionstore.ErrSourceNotRemoved):
		writeJSON(w, map[string]any{"path": moved, "originalRemoved": false})
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err)
	default:
		writeJSON(w, map[string]any{"path": moved, "originalRemoved": true})
	}
}

// knownWorkspaceDir answers with the declared project root the request names, or
// "". The request text is only compared with roots the hub already knows: it is
// never handed to the file system.
func (h *Hub) knownWorkspaceDir(target string) string {
	abs, err := filepath.Abs(target)
	if err != nil {
		return ""
	}
	for _, ref := range h.sessionRoots() {
		if ref.dir == abs {
			return ref.dir
		}
	}
	return ""
}
