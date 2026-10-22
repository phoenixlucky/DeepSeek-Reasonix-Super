package serve

import (
	"encoding/json"
	"net/http"
)

func (s *Server) planApprovalSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().PlanApprovalSettings())
}

// savePlanApprovalSettings rides the provider-edit grant: choosing auto lets a
// plan start executing without a click, so a networked client must not make
// that choice for the machine's owner.
func (s *Server) savePlanApprovalSettings(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "plan_approval.editing_disabled", "plan-approval editing is not enabled on this server", nil)
		return
	}
	var body struct {
		Auto *bool `json:"auto"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		badBody(w)
		return
	}
	if body.Auto == nil {
		refuse(w, http.StatusBadRequest, "plan_approval.no_choice", "auto is required", nil)
		return
	}
	if err := s.ctl().SavePlanApprovalSettings(*body.Auto); err != nil {
		saveFailed(w, http.StatusInternalServerError, "plan_approval.save_failed", err)
		return
	}
	writeJSON(w, s.ctl().PlanApprovalSettings())
}
