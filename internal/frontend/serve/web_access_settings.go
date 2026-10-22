package serve

import (
	"encoding/json"
	"errors"
	"net/http"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func (s *Server) registerWebAccessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /web-access-settings", s.webAccessSettings)
	mux.HandleFunc("POST /web-access-settings", s.saveWebAccessSettings)
	mux.HandleFunc("POST /web-access-settings/probe", s.probeSearxng)
}

func (s *Server) webAccessSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().WebAccessSettings())
}

func (s *Server) saveWebAccessSettings(w http.ResponseWriter, r *http.Request) {
	// A standing consent to be charged and the address queries go to are the
	// window's to set: no paired or network client decides them for the user.
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "web_access.editing_disabled", "web search and fetch settings editing is not enabled on this server", nil)
		return
	}
	var patch control.WebAccessPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&patch); err != nil || patch.Empty() {
		badBody(w)
		return
	}
	saved, err := s.ctl().SaveWebAccessSettings(patch)
	var unknown *config.UnknownWebBackendError
	switch {
	case errors.As(err, &unknown):
		refuse(w, http.StatusBadRequest, "web_access.unknown_backend", err.Error(), map[string]any{"backend": unknown.Name})
	case errors.Is(err, config.ErrBadSearxngURL):
		refuse(w, http.StatusBadRequest, "web_access.bad_searxng_url", err.Error(), nil)
	case err != nil:
		saveFailed(w, http.StatusInternalServerError, "web_access.save_failed", err)
	default:
		writeJSON(w, saved)
	}
}

// probeSearxng sends one test search to the SearXNG address already saved, so a
// window can say why the instance fails. The request names no address: what it
// reaches is what the user saved, and a window saves the field first.
func (s *Server) probeSearxng(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "web_access.editing_disabled", "web search and fetch settings editing is not enabled on this server", nil)
		return
	}
	probe, err := s.ctl().ProbeSearxng(r.Context())
	if errors.Is(err, config.ErrBadSearxngURL) {
		refuse(w, http.StatusBadRequest, "web_access.bad_searxng_url", err.Error(), nil)
		return
	}
	if err != nil {
		saveFailed(w, http.StatusInternalServerError, "web_access.probe_failed", err)
		return
	}
	writeJSON(w, probe)
}
