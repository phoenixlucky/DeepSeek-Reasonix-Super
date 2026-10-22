package serve

import (
	"encoding/json"
	"errors"
	"net/http"

	"reasonix/internal/session/control"
)

const (
	codeSideUnavailable = "side.unavailable"
	codeSideUnknown     = "side.unknown"
	codeSideBusy        = "side.busy"
	codeSideLimit       = "side.limit"
	codeSideEmpty       = "side.empty"
	codeSideFailed      = "side.failed"
)

func (s *Server) registerSideRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /side", s.openSide)
	mux.HandleFunc("POST /side/{id}/ask", s.askSide)
	mux.HandleFunc("DELETE /side/{id}", s.closeSide)
}

// openSide starts an empty side conversation. It is held by this runtime and
// gone with it; nothing about it is written anywhere.
func (s *Server) openSide(w http.ResponseWriter, _ *http.Request) {
	id, err := s.ctl().OpenSide()
	if err != nil {
		refuseSide(w, err)
		return
	}
	writeJSON(w, map[string]any{"id": id})
}

// askSide answers one question and holds the request until the answer is ready;
// a client that goes away takes the question with it.
func (s *Server) askSide(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question string   `json:"question"`
		Quotes   []string `json:"quotes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		badBody(w)
		return
	}
	answer, err := s.ctl().AskSide(r.Context(), r.PathValue("id"), req.Question, req.Quotes)
	if err != nil {
		refuseSide(w, err)
		return
	}
	writeJSON(w, map[string]any{"answer": answer})
}

func (s *Server) closeSide(w http.ResponseWriter, r *http.Request) {
	s.ctl().CloseSide(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func refuseSide(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, control.ErrSideUnavailable):
		refuse(w, http.StatusNotFound, codeSideUnavailable, "side questions are not available in this session", nil)
	case errors.Is(err, control.ErrSideUnknown):
		refuse(w, http.StatusNotFound, codeSideUnknown, "that side question is closed", nil)
	case errors.Is(err, control.ErrSideBusy):
		refuse(w, http.StatusConflict, codeSideBusy, "this side question is still being answered", nil)
	case errors.Is(err, control.ErrSideLimit):
		refuse(w, http.StatusConflict, codeSideLimit, "too many side questions are open", nil)
	case errors.Is(err, control.ErrSideEmpty):
		refuse(w, http.StatusBadRequest, codeSideEmpty, "a side question needs some words", nil)
	default:
		saveFailed(w, http.StatusBadGateway, codeSideFailed, err)
	}
}
