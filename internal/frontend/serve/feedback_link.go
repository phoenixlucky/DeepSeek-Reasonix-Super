package serve

import (
	"net/http"

	"reasonix/internal/platform/feedback"
)

type feedbackLinksView struct {
	Installs []feedback.LinkedInstall `json:"installs"`
}

func (s *Server) feedbackLinks(w http.ResponseWriter, r *http.Request) {
	got, err := s.ctl().FeedbackLinks(r.Context())
	if err != nil {
		refuseFeedback(w, err)
		return
	}
	if got == nil {
		got = []feedback.LinkedInstall{}
	}
	writeJSON(w, feedbackLinksView{Installs: got})
}

func (s *Server) feedbackLink(w http.ResponseWriter, r *http.Request) {
	if err := s.ctl().LinkFeedbackAccount(r.Context()); err != nil {
		refuseFeedback(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) feedbackUnlink(w http.ResponseWriter, r *http.Request) {
	if err := s.ctl().UnlinkFeedbackAccount(r.Context()); err != nil {
		refuseFeedback(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) feedbackUnlinkInstall(w http.ResponseWriter, r *http.Request) {
	if err := s.ctl().UnlinkFeedbackInstall(r.Context(), r.PathValue("id")); err != nil {
		refuseFeedback(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
