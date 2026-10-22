package serve

import (
	"net/http"
)

// feedbackBoard is the public contribution board, all releases or one. It needs
// no account, so a paired device may read it; what it answers when the service
// has none is available:false, not an error.
func (s *Server) feedbackBoard(w http.ResponseWriter, r *http.Request) {
	got, err := s.ctl().FeedbackBoard(r.Context(), r.URL.Query().Get("release"))
	if err != nil {
		refuseFeedback(w, err)
		return
	}
	writeJSON(w, got)
}

// feedbackBoardMe, feedbackBoardJoin and feedbackBoardLeave act for the account
// the window is signed in as, so a paired device cannot use them (the one list is
// hostOnlyRoutes) and each spends the account's assertion on its one call.
func (s *Server) feedbackBoardMe(w http.ResponseWriter, r *http.Request) {
	if refuseDevice(w, r) {
		return
	}
	got, err := s.ctl().FeedbackBoardMe(r.Context())
	if err != nil {
		refuseFeedback(w, err)
		return
	}
	writeJSON(w, got)
}

func (s *Server) feedbackBoardJoin(w http.ResponseWriter, r *http.Request) {
	if refuseDevice(w, r) {
		return
	}
	var req struct {
		DisplayName string `json:"displayName"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	got, err := s.ctl().JoinFeedbackBoard(r.Context(), req.DisplayName)
	if err != nil {
		refuseFeedback(w, err)
		return
	}
	writeJSON(w, got)
}

func (s *Server) feedbackBoardLeave(w http.ResponseWriter, r *http.Request) {
	if refuseDevice(w, r) {
		return
	}
	got, err := s.ctl().LeaveFeedbackBoard(r.Context())
	if err != nil {
		refuseFeedback(w, err)
		return
	}
	writeJSON(w, got)
}
