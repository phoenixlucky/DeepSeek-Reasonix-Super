package serve

import (
	"encoding/json"
	"net/http"
)

func (s *Server) defaultModeSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().DefaultModeSettings())
}

// saveDefaultModeSettings rides the provider-edit grant like the other writes to
// the user file, so a networked client does not change how the owner's
// conversations start.
func (s *Server) saveDefaultModeSettings(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "default_mode.editing_disabled", "default-mode editing is not enabled on this server", nil)
		return
	}
	var body struct {
		Plan *bool `json:"plan"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		badBody(w)
		return
	}
	if body.Plan == nil {
		refuse(w, http.StatusBadRequest, "default_mode.no_choice", "plan is required", nil)
		return
	}
	if err := s.ctl().SaveDefaultModeSettings(*body.Plan); err != nil {
		saveFailed(w, http.StatusInternalServerError, "default_mode.save_failed", err)
		return
	}
	writeJSON(w, s.ctl().DefaultModeSettings())
}
