package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"reasonix/internal/contract/event"
	"reasonix/internal/session/control"
	"reasonix/internal/state/sessionstore"
)

// branchAt copies the conversation up to the end of one turn into a new session
// and answers with its path; this controller stays on its own session, so the
// frontend opens the branch beside it.
func (s *Server) branchAt(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Turn *int   `json:"turn"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Turn == nil || *body.Turn < 0 {
		missingField(w, "turn")
		return
	}
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	path, err := s.ctl().BranchAt(*body.Turn, body.Name)
	if err != nil {
		refuseBranchAt(w, err)
		return
	}
	writeJSON(w, struct {
		Path string `json:"path"`
	}{Path: path})
}

func refuseBranchAt(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, control.ErrTurnRunning):
		busy(w, "conversation_branch.turn_running", err.Error(), nil)
	case errors.Is(err, control.ErrBranchUnavailable):
		refuse(w, http.StatusConflict, "conversation_branch.unavailable", err.Error(), nil)
	default:
		refuse(w, http.StatusInternalServerError, "conversation_branch.failed", err.Error(), nil)
	}
}

// Only explicit branches with saved source settings inherit those settings.
// Recovery copies and ordinary sessions keep their existing opening behavior.
func branchSettings(path string) (*sessionstore.BranchMeta, error) {
	if path == "" {
		return nil, nil
	}
	meta, ok, err := sessionstore.LoadBranchMeta(path)
	if err != nil {
		return nil, err
	}
	if !ok || meta.ParentID == "" || meta.Recovered || meta.Superseded || meta.Model == "" {
		return nil, nil
	}
	return &meta, nil
}

func (s *Server) branchModelAvailable(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	if s.resolver != nil {
		for _, descriptor := range s.resolver.Catalog() {
			if descriptor.Ref == ref || descriptor.Model == ref {
				return true
			}
		}
		return false
	}
	cfg, err := runtimeConfig(s.ctl())
	if err != nil {
		return false
	}
	_, ok := cfg.ResolveModel(ref)
	return ok
}

func (s *Server) notifyBranchModelFallback(saved string) {
	if s.branchModelFallback == saved {
		s.branchModelFallback = ""
	}
	sink := s.paneSink
	if sink == nil {
		sink = s.bc
	}
	if sink == nil {
		return
	}
	sink.Emit(event.Event{
		Kind: event.Notice, Level: event.LevelWarn,
		Code:   event.NoticeCodeBranchModelUnavailable,
		Text:   "The saved branch model is unavailable, so another configured model is in use.",
		Detail: fmt.Sprintf("saved branch model: %s", saved),
	})
}
