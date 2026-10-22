package serve

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

// openAuthorizationPage starts the machine's browser on the authorization page.
// The controller decides whether it may be called; this only says how.
var openAuthorizationPage = func(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	cmd.Env = os.Environ()
	return cmd.Start()
}

// mcpSignIn signs in to one MCP server. It answers when the sign-in ends, up to
// the controller's bound, and rides the provider-edit grant: it opens a browser
// on this machine and stores a credential, which a networked client must not do.
func (s *Server) mcpSignIn(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "mcp.editing_disabled", "MCP sign-in is not enabled for this client", nil)
		return
	}
	name, ok := decodeMCPName(w, r)
	if !ok {
		return
	}
	res, err := s.ctl().AuthorizeMCPServer(r.Context(), name, openAuthorizationPage)
	if err != nil {
		refuseMCPSignIn(w, name, err)
		return
	}
	writeJSON(w, map[string]any{"name": name, "state": control.MCPHealthReady, "host": res.Host, "tools": res.Tools})
}

// mcpUpgrade extends a signed-in server's access to what it asked for. It rides
// the same grant as sign-in, and the controller asks the person every time.
func (s *Server) mcpUpgrade(w http.ResponseWriter, r *http.Request) {
	if !s.grants.at(r).providerEdit {
		refuse(w, http.StatusForbidden, "mcp.editing_disabled", "MCP sign-in is not enabled for this client", nil)
		return
	}
	name, ok := decodeMCPName(w, r)
	if !ok {
		return
	}
	res, err := s.ctl().UpgradeMCPServerAccess(r.Context(), name, openAuthorizationPage)
	if err != nil {
		refuseMCPSignIn(w, name, err)
		return
	}
	writeJSON(w, map[string]any{"name": name, "state": control.MCPHealthReady, "host": res.Host, "tools": res.Tools})
}

func refuseMCPSignIn(w http.ResponseWriter, name string, err error) {
	switch {
	case errors.Is(err, control.ErrMCPSignInDeclined):
		refuse(w, http.StatusConflict, "mcp.signin_declined", "sign-in was declined", nil)
	case errors.Is(err, control.ErrMCPSignInIneligible):
		refuse(w, http.StatusBadRequest, "mcp.signin_ineligible", "this MCP server does not use OAuth sign-in", map[string]any{"name": name})
	case errors.Is(err, control.ErrMCPSignInBusy):
		refuse(w, http.StatusConflict, "mcp.signin_busy", "a sign-in for this MCP server is already open", map[string]any{"name": name})
	case errors.Is(err, control.ErrMCPSignInNoHost):
		refuse(w, http.StatusBadGateway, "mcp.signin_no_host", "the server's authorization page names no host", map[string]any{"name": name})
	case errors.Is(err, control.ErrMCPNoScopeUpgrade):
		refuse(w, http.StatusConflict, "mcp.no_scope_upgrade", "this MCP server has not asked for more access", map[string]any{"name": name})
	case errors.Is(err, control.ErrMCPSignInNoBrowser):
		refuse(w, http.StatusInternalServerError, "mcp.signin_no_browser", "this machine cannot open a browser", nil)
	case errors.Is(err, context.DeadlineExceeded):
		refuse(w, http.StatusGatewayTimeout, "mcp.signin_timed_out", "sign-in was not completed in time", map[string]any{"name": name})
	case errors.Is(err, context.Canceled):
		refuse(w, http.StatusConflict, "mcp.signin_cancelled", "sign-in was cancelled", map[string]any{"name": name})
	case errors.Is(err, control.ErrMCPApprovalOwed):
		refuse(w, http.StatusConflict, "mcp.approval_owed", err.Error(), nil)
	default:
		writeCatalogErrorOr(w, name, err)
	}
}

// writeCatalogErrorOr names an unknown server as such and reports any other
// failure as the dependency it is: the server's authorization side said no.
func writeCatalogErrorOr(w http.ResponseWriter, name string, err error) {
	var unknown *config.ServerNotFoundError
	if errors.As(err, &unknown) {
		writeCatalogError(w, err)
		return
	}
	refuse(w, http.StatusBadGateway, "mcp.signin_failed", err.Error(), map[string]any{"name": name, "detail": err.Error()})
}
