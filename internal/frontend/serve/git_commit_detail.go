package serve

import (
	"errors"
	"net/http"

	"reasonix/internal/platform/gitstatus"
)

// commitFilesView is the files one commit changed. Repo is false for a workspace
// that is not version-controlled, with no files and nothing wrong.
type commitFilesView struct {
	Repo      bool                   `json:"repo"`
	Files     []gitstatus.CommitFile `json:"files"`
	Truncated bool                   `json:"truncated"`
}

// commitFiles lists what one commit changed, so a history row can open into its
// files. The id is a query parameter a client supplies: gitstatus accepts only a
// well-formed id that names a commit, and each failure carries its own code
// because a frontend says different things for a typo and for a commit that is
// not in this repository.
func (s *Server) commitFiles(w http.ResponseWriter, r *http.Request) {
	repo, named := s.requestRepo(w, r)
	if !named {
		return
	}
	id := r.URL.Query().Get("id")
	files, truncated, ok, err := gitstatus.CommitFiles(r.Context(), repo, id)
	if refuseCommitRead(w, err, id) {
		return
	}
	writeJSON(w, commitFilesView{Repo: ok, Files: nonNil(files), Truncated: truncated})
}

// commitDiff is one path's diff within one commit.
func (s *Server) commitDiff(w http.ResponseWriter, r *http.Request) {
	repo, named := s.requestRepo(w, r)
	if !named {
		return
	}
	id, path := r.URL.Query().Get("id"), r.URL.Query().Get("path")
	text, truncated, err := gitstatus.CommitDiff(r.Context(), repo, id, path)
	if errors.Is(err, gitstatus.ErrPathOutsideTree) {
		refuse(w, http.StatusBadRequest, "changes.path_outside_tree", "that path is not inside the working tree", map[string]any{"path": path})
		return
	}
	if refuseCommitRead(w, err, id) {
		return
	}
	writeJSON(w, struct {
		Path      string `json:"path"`
		Diff      string `json:"diff"`
		Truncated bool   `json:"truncated"`
	}{Path: path, Diff: text, Truncated: truncated})
}

func refuseCommitRead(w http.ResponseWriter, err error, id string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, gitstatus.ErrBadCommit):
		refuse(w, http.StatusBadRequest, "git.commit_bad_id", "that is not a commit id", nil)
	case errors.Is(err, gitstatus.ErrNoSuchCommit):
		refuse(w, http.StatusNotFound, "git.commit_unknown", "this repository has no such commit", map[string]any{"id": id})
	default:
		refuse(w, http.StatusInternalServerError, "git.commit_read_failed", err.Error(), nil)
	}
	return true
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
