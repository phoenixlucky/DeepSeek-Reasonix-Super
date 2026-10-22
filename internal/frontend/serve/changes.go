package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"reasonix/internal/platform/gitcmd"
	"reasonix/internal/platform/gitrepos"
	"reasonix/internal/platform/gitstatus"
	"reasonix/internal/session/control"
)

// changes reports what the working tree currently differs by. A frontend that
// derives its pending-change list from tool events instead keeps showing a file
// the agent created and a shell command then deleted, because no event says so.
// Repo is false for a workspace that is not version-controlled, which is a
// fallback signal and not a failure.
func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	repo, named := s.requestRepo(w, r)
	if !named {
		return
	}
	list, ok, err := gitstatus.Status(r.Context(), repo)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if list == nil {
		list = []gitstatus.Change{}
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Repo    bool               `json:"repo"`
		Changes []gitstatus.Change `json:"changes"`
	}{Repo: ok, Changes: list})
}

type workspaceGitView struct {
	Repo      bool   `json:"repo"`
	Name      string `json:"name"`
	Branch    string `json:"branch"`
	Detached  bool   `json:"detached"`
	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Untracked int    `json:"untracked"`
	// Absent says why a marked folder has no repository: "refused" (git said no) or
	// "unavailable" (git did not answer); Detail is what git printed.
	Absent string `json:"absent,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// workspaceGit answers with the work tree's one-line identity. Repo is false
// for a workspace that is not version-controlled, as for /changes.
func (s *Server) workspaceGit(w http.ResponseWriter, r *http.Request) {
	repo, named := s.requestRepo(w, r)
	if !named {
		return
	}
	info, ok := gitstatus.Summary(r.Context(), repo)
	w.Header().Set("content-type", "application/json")
	view := workspaceGitView{
		Repo: ok, Name: info.Name, Branch: info.Branch, Detached: info.Detached,
		Added: info.Added, Removed: info.Removed, Untracked: info.Untracked,
	}
	if !ok {
		view.Absent, view.Detail = string(repo.Absent), repo.AbsentDetail
	}
	_ = json.NewEncoder(w).Encode(view)
}

// gitHistoryView is the commit graph's newest page. Repo is false for a
// workspace that is not version-controlled, with no commits and nothing wrong.
type gitHistoryView struct {
	Repo      bool               `json:"repo"`
	Commits   []gitstatus.Commit `json:"commits"`
	Truncated bool               `json:"truncated"`
}

// gitHistory answers with the newest commits of every branch, newest first,
// bounded by ?limit (default 100). Reading it changes nothing in the repository.
func (s *Server) gitHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			refuse(w, http.StatusBadRequest, "request.bad_value", "limit must be a positive number", nil)
			return
		}
		limit = n
	}
	repo, named := s.requestRepo(w, r)
	if !named {
		return
	}
	page, ok, err := gitstatus.History(r.Context(), repo, limit)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "git.history_failed", err.Error(), nil)
		return
	}
	if page.Commits == nil {
		page.Commits = []gitstatus.Commit{}
	}
	writeJSON(w, gitHistoryView{Repo: ok, Commits: page.Commits, Truncated: page.Truncated})
}

// changeDiff answers with one path's working-tree diff, so a reader can see a
// change without leaving the window for an editor. The path is a query
// parameter a client supplies, and gitstatus.Diff is what keeps it inside the
// tree — a refusal here is a bad request, not a broken repository, and the two
// carry different codes because a frontend has to say different things.
func (s *Server) changeDiff(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	repo, named := s.requestRepo(w, r)
	if !named {
		return
	}
	text, truncated, err := gitstatus.Diff(r.Context(), repo, path)
	if errors.Is(err, gitstatus.ErrPathOutsideTree) {
		refuse(w, http.StatusBadRequest, "changes.path_outside_tree",
			"that path is not inside the working tree", map[string]any{"path": path})
		return
	}
	if err != nil {
		refuse(w, http.StatusInternalServerError, "changes.diff_failed", err.Error(), nil)
		return
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Path      string `json:"path"`
		Diff      string `json:"diff"`
		Truncated bool   `json:"truncated"`
	}{Path: path, Diff: text, Truncated: truncated})
}

// workspaceRepo is the git identity the controller's session opened with. A
// controller that carries none answers for a workspace git is not asked about.
func workspaceRepo(ctl control.SessionAPI) gitcmd.Repo {
	if c, ok := ctl.(interface{ WorkspaceRepo() gitcmd.Repo }); ok {
		return c.WorkspaceRepo()
	}
	return gitcmd.Repo{Dir: ctl.WorkspaceRoot()}
}

// requestRepo is the repository a git read names with ?repo=, a directory below
// the workspace as /git/repos lists it; absent is the workspace's own. A name
// that is not listed is refused with its own code and nothing is read.
func (s *Server) requestRepo(w http.ResponseWriter, r *http.Request) (gitcmd.Repo, bool) {
	return s.repoNamed(w, r, r.URL.Query().Get("repo"))
}

func (s *Server) repoNamed(w http.ResponseWriter, r *http.Request, dir string) (gitcmd.Repo, bool) {
	ctl, ok := s.ctl().(interface {
		RepoFor(ctx context.Context, dir string) (gitcmd.Repo, error)
	})
	if !ok {
		if dir != "" {
			refuse(w, http.StatusNotFound, "git.repo_unknown", "that repository is not part of this workspace", nil)
			return gitcmd.Repo{}, false
		}
		return workspaceRepo(s.ctl()), true
	}
	repo, err := ctl.RepoFor(r.Context(), dir)
	if errors.Is(err, gitrepos.ErrNotListed) {
		refuse(w, http.StatusNotFound, "git.repo_unknown", "that repository is not part of this workspace", nil)
		return gitcmd.Repo{}, false
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return gitcmd.Repo{}, false
	}
	return repo, true
}

type gitRepoRow struct {
	// Dir is the repository's folder below the workspace; empty when the
	// workspace is itself the repository.
	Dir      string `json:"dir"`
	Name     string `json:"name"`
	Branch   string `json:"branch"`
	Detached bool   `json:"detached"`
}

// gitReposView is the repositories of the workspace.
type gitReposView struct {
	Repos     []gitRepoRow `json:"repos"`
	Truncated bool         `json:"truncated"`
}

// gitRepos lists the repositories of the workspace with the identity of each,
// which branch and no counts (a heading is all that is drawn from it): the workspace itself when it is a repository, otherwise the repositories
// found below it. Truncated says a search bound ended the look early.
func (s *Server) gitRepos(w http.ResponseWriter, r *http.Request) {
	ctl, ok := s.ctl().(interface {
		GitRepos(ctx context.Context) (gitrepos.Result, error)
	})
	if !ok {
		writeJSON(w, gitReposView{Repos: []gitRepoRow{}})
		return
	}
	found, err := ctl.GitRepos(r.Context())
	if err != nil {
		refuse(w, http.StatusInternalServerError, "git.repos_failed", err.Error(), nil)
		return
	}
	rows := make([]gitRepoRow, len(found.Repos))
	var reads sync.WaitGroup
	for i, f := range found.Repos {
		reads.Go(func() {
			info, _ := gitstatus.Identity(r.Context(), f.Repo)
			rows[i] = gitRepoRow{Dir: f.Dir, Name: info.Name, Branch: info.Branch, Detached: info.Detached}
		})
	}
	reads.Wait()
	writeJSON(w, gitReposView{Repos: rows, Truncated: found.Truncated})
}
