package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/surfacespec"
)

func refuseSurfaceAnswer(w http.ResponseWriter, err error) {
	var e *surfacespec.Error
	if errors.As(err, &e) {
		status := http.StatusBadRequest
		if errors.Is(err, surfacespec.ErrExpired) {
			status = http.StatusConflict
		}
		refuse(w, status, string(e.Code), "", map[string]any{"path": e.Path})
		return
	}
	writeErr(w, http.StatusInternalServerError, err)
}

// answer responds to an ask_request.
func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string            `json:"id"`
		Answers []event.AskAnswer `json:"answers"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, surfacespec.MaxSpecBytes+4096)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			refuseSurfaceAnswer(w, surfacespec.ErrLimitExceeded)
			return
		}
		badBody(w)
		return
	}
	if body.ID == "" {
		missingField(w, "id")
		return
	}
	if err := answerAs(s.ctl(), r, body.ID, body.Answers); err != nil {
		refuseSurfaceAnswer(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
