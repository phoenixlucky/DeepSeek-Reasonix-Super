package serve

import (
	"errors"
	"net/http"
	"strings"

	"reasonix/internal/session/control"
)

// mcpFollow records whether the user takes one server's tool definition changes
// without being asked. It lowers a defence the user chose to have, so it rides
// the provider-edit grant like the other writes that change what the machine
// accepts on its owner's behalf.
func (s *Server) mcpFollow(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "mcp.editing_disabled", "MCP settings cannot be changed by this client", nil)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Follow *bool  `json:"follow"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		missingField(w, "name")
		return
	}
	if body.Follow == nil {
		missingField(w, "follow")
		return
	}
	tools, err := s.ctl().SetMCPFollowToolChanges(name, *body.Follow)
	switch {
	case errors.Is(err, control.ErrMCPApprovalOwed):
		refuse(w, http.StatusConflict, "mcp.approval_owed", err.Error(), nil)
	case errors.Is(err, control.ErrMCPFollowUnwritable):
		saveFailed(w, http.StatusInternalServerError, "mcp.follow_unwritable", err)
	case err != nil:
		writeCatalogError(w, err)
	default:
		writeJSON(w, map[string]any{"name": name, "follow": *body.Follow, "tools": tools})
	}
}
