package serve

import (
	"encoding/json"
	"net/http"
	"slices"

	"reasonix/internal/contract/config"
)

func (s *Server) registerTurnNarrationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /turn-narration-settings", s.turnNarrationSettings)
	mux.HandleFunc("POST /turn-narration-settings", s.saveTurnNarrationSettings)
}

func (s *Server) turnNarrationSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().TurnNarrationSettings())
}

func (s *Server) saveTurnNarrationSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Level string `json:"level"`
	}
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body)
	if err != nil || !slices.Contains([]string{config.TurnNarrationBrief, config.TurnNarrationDetailed}, body.Level) {
		badBody(w)
		return
	}
	if err := s.ctl().SaveTurnNarrationSettings(body.Level); err != nil {
		saveFailed(w, http.StatusInternalServerError, "turn_narration.save_failed", err)
		return
	}
	writeJSON(w, s.ctl().TurnNarrationSettings())
}
