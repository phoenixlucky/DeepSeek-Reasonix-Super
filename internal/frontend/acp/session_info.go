package acp

import (
	"context"
	"encoding/json"

	"reasonix/internal/session/control"
)

const (
	sessionInfoSchemaVersion = 1
	sessionInfoMethod        = "_reasonix.io/session/info"
	sessionMCPServersMethod  = "_reasonix.io/session/mcpServers"
)

// ReasonixSessionInfo is what identifies a session to an editor beside its
// running state: which model and workspace it answers for and how it is set.
type ReasonixSessionInfo struct {
	SchemaVersion    int                     `json:"schemaVersion"`
	SessionID        string                  `json:"sessionId"`
	Label            string                  `json:"label"`
	ModelRef         string                  `json:"modelRef"`
	WorkspaceRoot    string                  `json:"workspaceRoot"`
	Scope            control.CapabilityScope `json:"scope"`
	AgentPreset      string                  `json:"agentPreset"`
	AutoApproveTools bool                    `json:"autoApproveTools"`
	Cache            ReasonixSessionCache    `json:"cache"`
}

// ReasonixSessionCache is the whole session's prompt-cache tally, steadier than
// a single turn's and unaffected by compaction.
type ReasonixSessionCache struct {
	HitTokens  int      `json:"hitTokens"`
	MissTokens int      `json:"missTokens"`
	HitRatio   *float64 `json:"hitRatio"`
}

// SessionMCPServersParams addresses one session, and optionally one server.
type SessionMCPServersParams struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name,omitempty"`
}

// ReasonixMCPServers lists the servers the session's configuration declares and
// whether each is on. It reports no command, argument or environment.
type ReasonixMCPServers struct {
	SchemaVersion int                        `json:"schemaVersion"`
	SessionID     string                     `json:"sessionId"`
	Servers       []ReasonixConfiguredServer `json:"servers"`
}

type ReasonixConfiguredServer struct {
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	LocalOverride bool   `json:"localOverride"`
	Pending       bool   `json:"pending"`
	Changed       bool   `json:"changed"`
	Tools         int    `json:"tools"`
}

func registerSessionInfoMethods(conn *Conn, s *service) {
	conn.Handle(sessionInfoMethod, s.sessionInfo)
	conn.Handle(sessionMCPServersMethod, s.sessionMCPServers)
}

func sessionInfoSnapshot(id string, ctrl acpController) ReasonixSessionInfo {
	hit, miss := ctrl.SessionCache()
	cache := ReasonixSessionCache{HitTokens: hit, MissTokens: miss}
	if total := hit + miss; total > 0 {
		ratio := float64(hit) / float64(total)
		cache.HitRatio = &ratio
	}
	return ReasonixSessionInfo{
		SchemaVersion: sessionInfoSchemaVersion, SessionID: id,
		Label: ctrl.Label(), ModelRef: ctrl.ModelRef(), WorkspaceRoot: ctrl.WorkspaceRoot(),
		Scope: ctrl.CapabilityScope(), AgentPreset: ctrl.AgentPreset(),
		AutoApproveTools: ctrl.AutoApproveTools(), Cache: cache,
	}
}

func (s *service) sessionInfo(_ context.Context, raw json.RawMessage) (any, error) {
	var p SessionStatusParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: sessionInfoMethod + ": " + err.Error()}
	}
	sess := s.session(p.SessionID)
	if sess == nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: sessionInfoMethod + ": unknown session " + p.SessionID}
	}
	return sessionInfoSnapshot(p.SessionID, sess.currentCtrl()), nil
}

func (s *service) sessionMCPServers(_ context.Context, raw json.RawMessage) (any, error) {
	var p SessionMCPServersParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: sessionMCPServersMethod + ": " + err.Error()}
	}
	sess := s.session(p.SessionID)
	if sess == nil {
		return nil, &RPCError{Code: ErrInvalidParams, Message: sessionMCPServersMethod + ": unknown session " + p.SessionID}
	}
	ctrl := sess.currentCtrl()
	rows := ctrl.ConfiguredMCPServers()
	out := ReasonixMCPServers{SchemaVersion: sessionInfoSchemaVersion, SessionID: p.SessionID, Servers: make([]ReasonixConfiguredServer, 0, len(rows))}
	for _, row := range rows {
		if p.Name != "" && row.Entry.Name != p.Name {
			continue
		}
		enabled := row.Enabled
		if p.Name != "" {
			if on, err := ctrl.MCPServerEnabled(p.Name); err == nil {
				enabled = on
			}
		}
		out.Servers = append(out.Servers, ReasonixConfiguredServer{
			Name: row.Entry.Name, Enabled: enabled, LocalOverride: row.LocalOverride,
			Pending: row.Pending, Changed: row.Changed, Tools: len(row.Tools),
		})
	}
	return out, nil
}
