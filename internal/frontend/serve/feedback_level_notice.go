package serve

import (
	"encoding/json"
	"net/http"
)

func (s *Server) registerFeedbackLevelNoticeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /feedback-level-notices", s.feedbackLevelNotices)
	mux.HandleFunc("POST /feedback-level-notices", s.saveFeedbackLevelNotices)
}

func (s *Server) feedbackLevelNotices(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().FeedbackLevelNotices())
}

func (s *Server) saveFeedbackLevelNotices(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Enabled == nil {
		badBody(w)
		return
	}
	if err := s.ctl().SaveFeedbackLevelNotices(*body.Enabled); err != nil {
		saveFailed(w, http.StatusInternalServerError, "feedback_level_notices.save_failed", err)
		return
	}
	writeJSON(w, s.ctl().FeedbackLevelNotices())
}

// feedbackPromotionAck records that the person was told about a level; only the
// person dismissing the note reaches it.
func (s *Server) feedbackPromotionAck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Level *int `json:"level"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Level == nil || *req.Level < 0 {
		badBody(w)
		return
	}
	if err := s.ctl().AckFeedbackPromotion(*req.Level); err != nil {
		refuseFeedback(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
