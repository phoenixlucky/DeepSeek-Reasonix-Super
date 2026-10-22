package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/base/i18n"
	"reasonix/internal/contract/mcpdiag"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/safety/permission"
)

// MCPSignInApprovalTool is the internal tool name of the card shown before a
// browser opens for an MCP server's sign-in.
const MCPSignInApprovalTool = permission.MCPSignIn

// MCPScopeUpgradeApprovalTool is the card for granting a server more access than
// it was given. It is its own tool because it offers no "remember": agreeing to
// visit a host is one decision and agreeing to widen what a server may do is
// another, and neither a remembered host nor a session grant answers it.
const MCPScopeUpgradeApprovalTool = "mcp_scope_upgrade"

const mcpSignInBound = 5 * time.Minute

var (
	// ErrMCPSignInIneligible is a server that cannot sign in with OAuth: not a
	// Streamable HTTP server, or one with credentials configured.
	ErrMCPSignInIneligible = errors.New("this MCP server does not use OAuth sign-in")
	// ErrMCPSignInDeclined is the user refusing to be sent to the authorization host.
	ErrMCPSignInDeclined = errors.New("sign-in was declined")
	// ErrMCPSignInBusy is a second sign-in for a server whose first is still open.
	ErrMCPSignInBusy = errors.New("a sign-in for this MCP server is already open")
	// ErrMCPSignInNoBrowser is a frontend that cannot open a browser.
	ErrMCPSignInNoBrowser = errors.New("this frontend cannot open a browser")
	// ErrMCPSignInNoHost is an authorization URL that names no host to ask about.
	ErrMCPSignInNoHost = errors.New("the authorization page names no host")
	// ErrMCPNoScopeUpgrade is a request to extend access when no server asked for it.
	ErrMCPNoScopeUpgrade = plugin.ErrNoScopeUpgradePending
)

// MCPSignInResult is a finished sign-in: the host the user was sent to and the
// tool count of the server once it reconnected.
type MCPSignInResult struct {
	Server string `json:"server"`
	Host   string `json:"host"`
	Tools  int    `json:"tools"`
}

// AuthorizeMCPServer signs in to one configured MCP server. The frontend only
// supplies how a browser is opened; whether it is opened at all is decided here,
// after the user has seen which host the server sends them to. A browser is
// never opened from a tool call or a background failure, only from this verb.
func (c *Controller) AuthorizeMCPServer(ctx context.Context, name string, open func(url string) error) (MCPSignInResult, error) {
	return c.authorizeAndReconnect(ctx, name, open, false)
}

// UpgradeMCPServerAccess extends what a signed-in server may do, to the scopes it
// asked for when it refused a call. Only the user starts it, and the card names
// the scopes the server sent as that server's words; it is asked every time.
func (c *Controller) UpgradeMCPServerAccess(ctx context.Context, name string, open func(url string) error) (MCPSignInResult, error) {
	return c.authorizeAndReconnect(ctx, name, open, true)
}

func (c *Controller) authorizeAndReconnect(ctx context.Context, name string, open func(url string) error, upgrade bool) (MCPSignInResult, error) {
	result, err := c.authorizeMCPServerWith(ctx, name, open, upgrade)
	if err != nil {
		return MCPSignInResult{}, err
	}
	if result.Tools, err = c.ReconnectMCPServer(result.Server); err != nil {
		return result, err
	}
	return result, nil
}

// authorizeMCPServer is the sign-in itself, up to the stored token; the caller
// reconnects the server with it.
func (c *Controller) authorizeMCPServer(ctx context.Context, name string, open func(url string) error) (MCPSignInResult, error) {
	return c.authorizeMCPServerWith(ctx, name, open, false)
}

func (c *Controller) authorizeMCPServerWith(ctx context.Context, name string, open func(url string) error, upgrade bool) (MCPSignInResult, error) {
	name = strings.TrimSpace(name)
	entry, err := c.configuredMCPServer(name)
	if err != nil {
		return MCPSignInResult{}, err
	}
	if !mcpdiag.CanUseHTTPMCPOAuth(entry.Type, entry.URL, mcpdiag.HasAuthConfig(entry.Headers, entry.Env, entry.URL)) {
		return MCPSignInResult{}, ErrMCPSignInIneligible
	}
	if open == nil {
		return MCPSignInResult{}, ErrMCPSignInNoBrowser
	}
	if !c.mcp.beginSignIn(name) {
		return MCPSignInResult{}, ErrMCPSignInBusy
	}
	defer c.mcp.endSignIn(name)

	ctx, cancel := context.WithTimeout(ctx, mcpSignInBound)
	defer cancel()
	spec := c.mcpSpec(entry)
	spec.OAuthScopeUpgrade = upgrade
	asked := plugin.PendingScopeUpgrade(spec.StateDir)
	if upgrade && len(asked) == 0 {
		return MCPSignInResult{}, ErrMCPNoScopeUpgrade
	}
	result := MCPSignInResult{Server: name}
	err = plugin.AuthorizeHTTPMCP(ctx, spec, func(authURL string) error {
		host := plugin.AuthorizationHost(authURL)
		if upgrade {
			if err := c.consentMCPScopeUpgrade(ctx, name, host, asked); err != nil {
				return err
			}
		} else if err := c.consentMCPSignIn(ctx, name, host, spec.StateDir); err != nil {
			return err
		}
		result.Host = host
		return open(authURL)
	})
	if err != nil {
		return MCPSignInResult{}, err
	}
	return result, nil
}

// consentMCPSignIn asks before the browser opens, unless the user already agreed
// to this host for this server. "Always" is kept beside the server's own OAuth
// state, so a different server sending the user to the same host is asked again.
func (c *Controller) consentMCPSignIn(ctx context.Context, server, host, stateDir string) error {
	if host == "" {
		return ErrMCPSignInNoHost
	}
	if plugin.SignInHostRemembered(stateDir, host) {
		return nil
	}
	subject := server + " → " + host
	if c.approval.preApprovedForDecision(MCPSignInApprovalTool, subject, nil, true) {
		return nil
	}
	args, _ := json.Marshal(map[string]string{"server": server, "host": host})
	reply, err := c.requestApprovalDecision(ctx, approvalRequest{tool: MCPSignInApprovalTool, subject: subject, args: args,
		reason: fmt.Sprintf(i18n.M.MCPSignInReasonFmt, server, host), fresh: true})
	if err != nil {
		return err
	}
	if !reply.allow {
		return ErrMCPSignInDeclined
	}
	if reply.session || reply.persist {
		c.approval.grantSession(MCPSignInApprovalTool, subject)
	}
	if reply.persist {
		c.emitRememberResult(rememberMCPSignInHost(server, host, stateDir))
	}
	return nil
}

// consentMCPScopeUpgrade asks before the browser opens to grant a server more
// access. It is asked every time, in every approval mode: no remembered host and
// no session grant stands in for it, and the card offers neither.
func (c *Controller) consentMCPScopeUpgrade(ctx context.Context, server, host string, scopes []string) error {
	if host == "" {
		return ErrMCPSignInNoHost
	}
	args, _ := json.Marshal(map[string]any{"server": server, "host": host, "scopes": scopes})
	reply, err := c.requestApprovalDecision(ctx, approvalRequest{tool: MCPScopeUpgradeApprovalTool, subject: server + " → " + host, args: args,
		reason: fmt.Sprintf(i18n.M.MCPScopeUpgradeReasonFmt, server, host, strings.Join(scopes, " ")), fresh: true})
	if err != nil {
		return err
	}
	if !reply.allow {
		return ErrMCPSignInDeclined
	}
	return nil
}

func rememberMCPSignInHost(server, host, stateDir string) RememberResult {
	rule := "mcp sign-in " + server + " += " + host
	if err := plugin.RememberSignInHost(stateDir, host); err != nil {
		return RememberResult{Rule: rule, Err: err}
	}
	return RememberResult{Rule: rule, Saved: true}
}

func (m *mcpManager) beginSignIn(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.signingIn[name] {
		return false
	}
	if m.signingIn == nil {
		m.signingIn = map[string]bool{}
	}
	m.signingIn[name] = true
	return true
}

func (m *mcpManager) endSignIn(name string) {
	m.mu.Lock()
	delete(m.signingIn, name)
	m.mu.Unlock()
}
