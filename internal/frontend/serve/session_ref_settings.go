package serve

import (
	"encoding/json"
	"errors"
	"net/http"

	"reasonix/internal/contract/config"
)

func (s *Server) registerSessionRefRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /session-ref-settings", s.sessionRefSettings)
	mux.HandleFunc("POST /session-ref-settings", s.saveSessionRefSettings)
}

func (s *Server) sessionRefSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().SessionRefSettings())
}

// saveSessionRefSettings needs no rebuild: each reference reads the limit as it
// is resolved. A body without the number is refused, never read as the default.
func (s *Server) saveSessionRefSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tokens *int `json:"tokens"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Tokens == nil {
		badBody(w)
		return
	}
	if err := s.ctl().SaveSessionRefSettings(*body.Tokens); err != nil {
		if errors.Is(err, config.ErrSessionRefTokensOutOfRange) {
			saveFailed(w, http.StatusBadRequest, "session_ref.out_of_range", err)
			return
		}
		saveFailed(w, http.StatusInternalServerError, "session_ref.save_failed", err)
		return
	}
	writeJSON(w, s.ctl().SessionRefSettings())
}
