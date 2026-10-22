package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"reasonix/internal/ext/skill"
	"reasonix/internal/session/control"
)

func (s *Server) registerSkillRemoveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /skills/remove", s.skillRemove)
}

// skillRemove deletes the skill the person was shown. The request names the
// scope and the path the list drew, so a skill that has since changed under the
// same name is refused instead of removed. Deleting files on this machine is the
// window's alone: a paired device reaches the pane's routes, so it is refused
// here. Another project's list is read, not edited, from this pane.
func (s *Server) skillRemove(w http.ResponseWriter, r *http.Request) {
	if !hostReach(r) {
		refuse(w, http.StatusNotFound, codeDeviceHostOnly, "a paired device cannot use this", nil)
		return
	}
	var body struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
		Path  string `json:"path"`
		Root  string `json:"root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badBody(w)
		return
	}
	name, scope, path := strings.TrimSpace(body.Name), strings.TrimSpace(body.Scope), strings.TrimSpace(body.Path)
	for field, v := range map[string]string{"name": name, "scope": scope, "path": path} {
		if v == "" {
			missingField(w, field)
			return
		}
	}
	root, other, ok := s.resolveRoot(body.Root)
	if !ok {
		unknownProject(w, root)
		return
	}
	if other {
		refuse(w, http.StatusConflict, "skill.other_project", "a skill of another project is not removed from this pane", map[string]any{"root": root})
		return
	}
	switch err := s.ctl().RemoveSkill(name, skill.Scope(scope), path); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, control.ErrSkillNotRemovable):
		refuse(w, http.StatusConflict, "skill.not_removable", err.Error(), nil)
	case errors.Is(err, control.ErrSkillChanged):
		refuse(w, http.StatusConflict, "skill.changed", err.Error(), nil)
	default:
		var unknown *skill.NotFoundError
		if errors.As(err, &unknown) {
			notFound(w, "skill", unknown.Name)
			return
		}
		refuse(w, http.StatusInternalServerError, "skill.remove_failed", err.Error(), map[string]any{"detail": err.Error()})
	}
}
