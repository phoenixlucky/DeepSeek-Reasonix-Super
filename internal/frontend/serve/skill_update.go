package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/ext/skillupdate"
)

// Why a skill update did not happen. Each is a different thing for the person
// to do next, so none shares a code.
const (
	codeSkillUpdateBadName    = "skills.update_bad_name"
	codeSkillUpdateNotRecord  = "skills.update_not_recorded"
	codeSkillUpdateNotLocal   = "skills.update_not_local"
	codeSkillUpdateGone       = "skills.update_source_missing"
	codeSkillUpdateStale      = "skills.update_plan_stale"
	codeSkillUpdateConfirm    = "skills.update_needs_confirm"
	codeSkillUpdateTooLarge   = "skills.update_too_large"
	codeSkillUpdateFailed     = "skills.update_failed"
	codeSkillUpdateBodyNoPlan = "skills.update_plan_required"
)

// An update of an installed skill is the person's act: these are window routes,
// not tools, and like every mutation they need the launch token the agent never
// holds. The plan is a POST for the same reason.
func (s *Server) registerSkillUpdateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /skills/{name}/update/plan", s.planSkillUpdate)
	mux.HandleFunc("POST /skills/{name}/update/apply", s.applySkillUpdate)
}

func (s *Server) planSkillUpdate(w http.ResponseWriter, r *http.Request) {
	plan, err := skillupdate.Updater{Home: config.ReasonixHomeDir()}.Plan(strings.TrimSpace(r.PathValue("name")))
	if err != nil {
		refuseSkillUpdate(w, err)
		return
	}
	writeJSON(w, plan)
}

func (s *Server) applySkillUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PlanID         string `json:"planId"`
		OverwriteLocal bool   `json:"overwriteLocal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badBody(w)
		return
	}
	if strings.TrimSpace(body.PlanID) == "" {
		refuse(w, http.StatusBadRequest, codeSkillUpdateBodyNoPlan, "an update applies the plan that was shown; planId is required", nil)
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if err := (skillupdate.Updater{Home: config.ReasonixHomeDir()}).Apply(name, body.PlanID, body.OverwriteLocal); err != nil {
		refuseSkillUpdate(w, err)
		return
	}
	out := map[string]any{"name": name, "updated": true}
	if err := s.reloadExtensions(r.Context()); err != nil {
		out["reloadError"] = err.Error()
	}
	writeJSON(w, out)
}

func refuseSkillUpdate(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skillupdate.ErrBadName):
		refuse(w, http.StatusBadRequest, codeSkillUpdateBadName, err.Error(), nil)
	case errors.Is(err, skillupdate.ErrNotRecorded):
		refuse(w, http.StatusNotFound, codeSkillUpdateNotRecord, err.Error(), nil)
	case errors.Is(err, skillupdate.ErrNotLocal):
		refuse(w, http.StatusConflict, codeSkillUpdateNotLocal, err.Error(), nil)
	case errors.Is(err, skillupdate.ErrSourceMissing):
		refuse(w, http.StatusConflict, codeSkillUpdateGone, err.Error(), nil)
	case errors.Is(err, skillupdate.ErrStale):
		refuse(w, http.StatusConflict, codeSkillUpdateStale, err.Error(), nil)
	case errors.Is(err, skillupdate.ErrNeedsConfirm):
		refuse(w, http.StatusConflict, codeSkillUpdateConfirm, err.Error(), nil)
	case errors.Is(err, skillupdate.ErrTooLarge):
		refuse(w, http.StatusRequestEntityTooLarge, codeSkillUpdateTooLarge, err.Error(), nil)
	default:
		saveFailed(w, http.StatusInternalServerError, codeSkillUpdateFailed, err)
	}
}
