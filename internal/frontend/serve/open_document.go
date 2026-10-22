package serve

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/platform/opendoc"
)

const (
	codeDocumentNoWindow     = "document.no_window"
	codeDocumentOutsideRoots = "document.outside_roots"
	codeDocumentType         = "document.not_allowed_type"
	codeDocumentNotFile      = "document.not_file"
	codeDocumentMissing      = "document.not_found"
	codeDocumentLaunch       = "document.launch_failed"
)

var launchDocument = opendoc.Open

// openableDocument lists the types the system opener may be handed. Each is a
// document its default program renders, never something that runs as a program.
var openableDocument = map[string]bool{
	".html": true, ".htm": true, ".xhtml": true, ".svg": true, ".pdf": true,
	".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
	".md": true, ".markdown": true, ".txt": true, ".csv": true, ".tsv": true,
}

// openDocument hands one file to the machine's default program. The file must
// lie in the workspace or a granted folder and, once links are followed, be a
// regular file of a listed type: the type that counts is the real file's.
func (s *Server) openDocument(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).localDesktop {
		refuse(w, http.StatusForbidden, codeDocumentNoWindow, "this kernel has no window to open a document from", nil)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
		refuse(w, http.StatusBadRequest, "request.bad_body", "path is required", nil)
		return
	}
	workspace := s.ctl().WorkspaceRoot()
	candidate := req.Path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(workspace, candidate)
	}
	roots := []string{workspace}
	if extra, err := s.ctl().ReadableExtraRoots(); err == nil {
		roots = append(roots, extra...)
	}
	named, ok := lexicallyWithin(roots, filepath.Clean(candidate))
	if !ok {
		refuse(w, http.StatusForbidden, codeDocumentOutsideRoots, "the file is outside the workspace and its granted folders", nil)
		return
	}
	real, err := filepath.EvalSymlinks(named)
	if errors.Is(err, fs.ErrNotExist) {
		refuse(w, http.StatusNotFound, codeDocumentMissing, "the file does not exist", nil)
		return
	}
	if err != nil || !withinAny(roots, real) {
		refuse(w, http.StatusForbidden, codeDocumentOutsideRoots, "the file resolves outside the workspace and its granted folders", nil)
		return
	}
	if info, err := os.Stat(real); err != nil {
		refuse(w, http.StatusNotFound, codeDocumentMissing, "the file does not exist", nil)
		return
	} else if !info.Mode().IsRegular() {
		refuse(w, http.StatusBadRequest, codeDocumentNotFile, "only a file can be opened", nil)
		return
	}
	ext := strings.ToLower(filepath.Ext(real))
	if !openableDocument[ext] {
		refuse(w, http.StatusBadRequest, codeDocumentType, "this type of file is not opened from here", map[string]any{"ext": ext})
		return
	}
	if err := launchDocument(real); err != nil {
		refuse(w, http.StatusInternalServerError, codeDocumentLaunch, err.Error(), nil)
		return
	}
	writeJSON(w, map[string]any{"path": real})
}

// lexicallyWithin is the string-only first gate, run before any file is
// touched. It tries each root as written and as its links resolve, so a path
// spelled either way passes, and answers the path rebuilt from that root.
func lexicallyWithin(roots []string, candidate string) (string, bool) {
	for _, root := range roots {
		forms := []string{filepath.Clean(root)}
		if real, err := filepath.EvalSymlinks(root); err == nil {
			forms = append(forms, real)
		}
		for _, form := range forms {
			if rel, err := filepath.Rel(form, candidate); err == nil && filepath.IsLocal(rel) {
				return filepath.Join(form, rel), true
			}
		}
	}
	return "", false
}

// withinAny judges the resolved file against each root's own resolved form, so
// a link anywhere on either side cannot make the lexical comparison lie.
func withinAny(roots []string, resolved string) bool {
	for _, root := range roots {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(real, resolved); err == nil && filepath.IsLocal(rel) {
			return true
		}
	}
	return false
}
