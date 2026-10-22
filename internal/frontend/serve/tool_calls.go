package serve

import "net/http"

func (s *Server) registerToolCallRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /tool-calls/{id}/cancel", s.cancelToolCall)
}

// cancelToolCall stops one running tool call; the turn carries on. A call that
// already finished is refused rather than acknowledged, so the card showing a
// stop button for it is told it is out of date.
func (s *Server) cancelToolCall(w http.ResponseWriter, r *http.Request) {
	if !s.ctl().CancelToolCall(r.PathValue("id")) {
		refuse(w, http.StatusConflict, "tool_call.not_running", "no running tool call with this id in this session", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
