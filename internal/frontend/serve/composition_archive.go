package serve

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"reasonix/internal/state/sessionstore"
)

func (h *Hub) importNativeSession(w http.ResponseWriter, r *http.Request) {
	workspace, content, err := readNativeUpload(w, r)
	if err != nil {
		refuseNativeArchive(w, err)
		return
	}
	if strings.TrimSpace(workspace) == "" {
		missingField(w, "workspace")
		return
	}
	root, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	root = h.knownSessionWorkspace(root)
	if root == "" {
		refuse(w, http.StatusForbidden, "workspace.unknown", "select a known workspace", nil)
		return
	}
	root, err = resolveWorkspaceDir(root)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	session, err := sessionstore.DecodeNativeArchive(content)
	if err != nil {
		refuseNativeArchive(w, err)
		return
	}
	path := sessionstore.NewSessionPath(SessionDirFor(root), "imported-agent")
	if err := session.SaveIfAbsent(path); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"path": path, "agentComposition": compositionView(session.Composition())})
}

func (h *Hub) knownSessionWorkspace(asked string) string {
	for _, ref := range h.sessionRoots() {
		if known, err := filepath.Abs(ref.dir); err == nil && known == asked {
			return ref.dir
		}
	}
	return ""
}

func readNativeUpload(w http.ResponseWriter, r *http.Request) (string, []byte, error) {
	if r.URL.Query().Has("workspace") {
		if r.ContentLength > sessionstore.MaxNativeArchiveBytes {
			return "", nil, sessionstore.ErrSessionReplayLimitExceeded
		}
		content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, sessionstore.MaxNativeArchiveBytes))
		return r.URL.Query().Get("workspace"), content, err
	}
	var body struct {
		Workspace string          `json:"workspace"`
		Content   json.RawMessage `json:"content"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, sessionstore.MaxNativeArchiveBytes+4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return "", nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", nil, sessionstore.ErrNativeArchiveInvalid
	}
	return body.Workspace, body.Content, nil
}

func refuseNativeArchive(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, sessionstore.ErrSessionReplayLimitExceeded), errors.As(err, &tooLarge):
		refuse(w, http.StatusRequestEntityTooLarge, "session.archive_too_large", "session archive exceeds safe limits", nil)
	case errors.Is(err, sessionstore.ErrNativeArchiveUnsupported):
		refuse(w, http.StatusBadRequest, "session.archive_unsupported", "unsupported session archive version", nil)
	default:
		refuse(w, http.StatusBadRequest, "session.archive_invalid", "invalid session archive", nil)
	}
}
