package acp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"reasonix/internal/session/control"
)

const (
	sessionControlSchemaVersion = 1
	sessionMCPReconnectMethod   = "_reasonix.io/session/mcp/reconnect"
	sessionMCPDisconnectMethod  = "_reasonix.io/session/mcp/disconnect"
	sessionExtensionSubmitForm  = "_reasonix.io/session/extension/submitForm"
)

// The states a reconnect answers with. A server the project declared and the
// user has not approved is refused with what it would run, never started.
const (
	mcpControlReady        = "ready"
	mcpControlFailed       = "failed"
	mcpControlApprovalOwed = "approval_owed"
	codeMCPApprovalOwed    = "mcp.approval_owed"
)

// SessionMCPNameParams addresses one configured server of one session.
type SessionMCPNameParams struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
}

// ReasonixMCPReconnect is a reconnect's outcome. Code and Launch are set only
// when approval is owed: Launch is the command the server would run.
type ReasonixMCPReconnect struct {
	SchemaVersion int    `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	Name          string `json:"name"`
	State         string `json:"state"`
	Tools         int    `json:"tools,omitempty"`
	Code          string `json:"code,omitempty"`
	Launch        string `json:"launch,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ReasonixMCPDisconnect reports whether a live server was disconnected.
type ReasonixMCPDisconnect struct {
	SchemaVersion int    `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	Name          string `json:"name"`
	Disconnected  bool   `json:"disconnected"`
}

// SessionExtensionFormParams carries one extension form's values back to the
// extension that owns the surface.
type SessionExtensionFormParams struct {
	SessionID string         `json:"sessionId"`
	PluginID  string         `json:"pluginId"`
	SurfaceID string         `json:"surfaceId"`
	Values    map[string]any `json:"values"`
}

// ReasonixExtensionFormResult says the extension accepted the submission.
type ReasonixExtensionFormResult struct {
	SchemaVersion int    `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	Accepted      bool   `json:"accepted"`
}

func registerSessionControlMethods(conn *Conn, s *service) {
	conn.Handle(sessionMCPReconnectMethod, s.sessionMCPReconnect)
	conn.Handle(sessionMCPDisconnectMethod, s.sessionMCPDisconnect)
	conn.Handle(sessionExtensionSubmitForm, s.sessionExtensionSubmitForm)
}

func (s *service) namedMCPServer(method string, raw json.RawMessage) (*acpSession, string, error) {
	var p SessionMCPNameParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, "", &RPCError{Code: ErrInvalidParams, Message: method + ": " + err.Error()}
	}
	sess := s.session(p.SessionID)
	if sess == nil {
		return nil, "", &RPCError{Code: ErrInvalidParams, Message: method + ": unknown session " + p.SessionID}
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, "", &RPCError{Code: ErrInvalidParams, Message: method + ": name is required"}
	}
	return sess, name, nil
}

// sessionMCPReconnect retries one configured server for this session. It goes
// through the controller's own approval check, so a project-declared server
// stays off until the user enables it where its command is shown.
func (s *service) sessionMCPReconnect(_ context.Context, raw json.RawMessage) (any, error) {
	sess, name, err := s.namedMCPServer(sessionMCPReconnectMethod, raw)
	if err != nil {
		return nil, err
	}
	sess.stateChangeMu.Lock()
	tools, connErr := sess.currentCtrl().ReconnectMCPServer(name)
	sess.stateChangeMu.Unlock()
	out := ReasonixMCPReconnect{SchemaVersion: sessionControlSchemaVersion, SessionID: sess.id, Name: name}
	var owed *control.MCPApprovalOwedError
	switch {
	case errors.As(connErr, &owed):
		out.State, out.Code, out.Launch = mcpControlApprovalOwed, codeMCPApprovalOwed, owed.Launch
	case connErr != nil:
		out.State, out.Error = mcpControlFailed, connErr.Error()
	default:
		out.State, out.Tools = mcpControlReady, tools
	}
	s.publishMCPStatus(sess)
	return out, nil
}

// sessionMCPDisconnect turns one server off for this session only; nothing is
// written to configuration or to the user's activation choices.
func (s *service) sessionMCPDisconnect(_ context.Context, raw json.RawMessage) (any, error) {
	sess, name, err := s.namedMCPServer(sessionMCPDisconnectMethod, raw)
	if err != nil {
		return nil, err
	}
	sess.stateChangeMu.Lock()
	disconnected := sess.currentCtrl().DisconnectMCPServer(name)
	sess.stateChangeMu.Unlock()
	s.publishMCPStatus(sess)
	return ReasonixMCPDisconnect{SchemaVersion: sessionControlSchemaVersion, SessionID: sess.id, Name: name, Disconnected: disconnected}, nil
}

func (s *service) sessionExtensionSubmitForm(ctx context.Context, raw json.RawMessage) (any, error) {
	var p SessionExtensionFormParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: sessionExtensionSubmitForm + ": " + err.Error()}
	}
	sess := s.session(p.SessionID)
	if sess == nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: sessionExtensionSubmitForm + ": unknown session " + p.SessionID}
	}
	if strings.TrimSpace(p.PluginID) == "" || strings.TrimSpace(p.SurfaceID) == "" {
		return nil, &RPCError{Code: ErrInvalidParams, Message: sessionExtensionSubmitForm + ": pluginId and surfaceId are required"}
	}
	if err := sess.currentCtrl().SubmitExtensionForm(ctx, p.PluginID, p.SurfaceID, p.Values); err != nil {
		return nil, &RPCError{Code: ErrInternal, Message: sessionExtensionSubmitForm + ": " + err.Error()}
	}
	return ReasonixExtensionFormResult{SchemaVersion: sessionControlSchemaVersion, SessionID: sess.id, Accepted: true}, nil
}
