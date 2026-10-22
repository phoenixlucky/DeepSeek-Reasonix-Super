package serve

import (
	"encoding/json"
	"errors"
	"net/http"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

func (s *Server) registerObservabilityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /observability", s.observabilitySettings)
	mux.HandleFunc("POST /observability", s.saveObservabilitySettings)
}

func (s *Server) observabilitySettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.ctl().ObservabilitySettings())
}

// saveObservabilitySettings rides the provider-edit grant: turning the export
// on or moving it decides where a run's structure is sent, which a networked
// client must not do.
func (s *Server) saveObservabilitySettings(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "observability.editing_disabled", "trace export settings are not editable by this client", nil)
		return
	}
	var body struct {
		Enabled     *bool   `json:"enabled"`
		Endpoint    *string `json:"endpoint"`
		HeaderName  *string `json:"headerName"`
		HeaderEnv   *string `json:"headerEnv"`
		ServiceName *string `json:"serviceName"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		badBody(w)
		return
	}
	got, err := s.ctl().SaveObservabilitySettings(control.ObservabilityPatch{
		Enabled: body.Enabled, Endpoint: body.Endpoint, HeaderName: body.HeaderName, HeaderEnv: body.HeaderEnv, ServiceName: body.ServiceName})
	if err != nil {
		refuseObservability(w, err)
		return
	}
	writeJSON(w, got)
}

func refuseObservability(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, config.ErrOTLPEndpointInvalid):
		refuse(w, http.StatusBadRequest, "observability.endpoint_invalid", "the endpoint must be an http or https address with a host", nil)
	case errors.Is(err, config.ErrOTLPEndpointCredentials):
		refuse(w, http.StatusBadRequest, "observability.endpoint_credentials", "the endpoint must not carry credentials, a query or a fragment", nil)
	case errors.Is(err, config.ErrOTLPHeaderInvalid):
		refuse(w, http.StatusBadRequest, "observability.header_invalid", "that is not a usable header name", nil)
	case errors.Is(err, config.ErrOTLPHeaderEnvInvalid):
		refuse(w, http.StatusBadRequest, "observability.header_env_invalid", "the credential variable must start with OTEL_ or REASONIX_OTEL_", nil)
	default:
		saveFailed(w, http.StatusInternalServerError, "observability.save_failed", err)
	}
}
