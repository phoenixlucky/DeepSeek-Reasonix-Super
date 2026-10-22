package acp

import (
	"strings"

	"reasonix/internal/session/control"
)

const (
	// CodeToolApproval is the data.code of a refusal about the session's tool
	// approval posture.
	CodeToolApproval             = "acp.tool_approval"
	codeToolApprovalAboveCeiling = CodeToolApproval + ".above_ceiling"
)

// PostureCeiling is implemented by a Factory started with a fixed tool approval
// posture. The posture is the most permissive one a client may select; an empty
// string leaves sessions uncapped.
type PostureCeiling interface {
	PostureCeiling() string
}

// ToolApprovalData is the data of an invalid-params error about a posture: a
// stable code and the ceiling the client ran into.
type ToolApprovalData struct {
	Code    string `json:"code"`
	Ceiling string `json:"ceiling,omitempty"`
}

// approvalPosture is a session's tool approval posture. kept is the higher one
// its file held when mode was cut to the ceiling; the file keeps it until the
// client chooses a posture itself.
type approvalPosture struct{ mode, kept string }

var toolApprovalOrder = []string{
	control.ToolApprovalReadOnly, control.ToolApprovalDontAsk, control.ToolApprovalAsk,
	control.ToolApprovalAuto, control.ToolApprovalYolo,
}

func (s *service) postureCeiling() string {
	c, ok := s.factory.(PostureCeiling)
	if !ok {
		return ""
	}
	if v := strings.TrimSpace(c.PostureCeiling()); v != "" {
		return normalizeACPToolApprovalMode(v)
	}
	return ""
}

// startingPosture is where a new session opens: the ceiling when one is fixed.
func (s *service) startingPosture() string {
	if ceiling := s.postureCeiling(); ceiling != "" {
		return ceiling
	}
	return control.ToolApprovalAsk
}

// restoredPosture is the posture an opened session file resumes in, and the
// higher one it held when that was cut to the ceiling ("" otherwise).
func (s *service) restoredPosture(saved string) (mode, kept string) {
	mode = clampToCeiling(saved, s.postureCeiling())
	if saved = normalizeACPToolApprovalMode(saved); saved != mode {
		return mode, saved
	}
	return mode, ""
}

func toolApprovalRank(mode string) int {
	mode = normalizeACPToolApprovalMode(mode)
	for i, m := range toolApprovalOrder {
		if m == mode {
			return i
		}
	}
	return 0
}

func withinCeiling(mode, ceiling string) bool {
	return ceiling == "" || toolApprovalRank(mode) <= toolApprovalRank(ceiling)
}

// clampToCeiling returns mode, or the ceiling when mode is above it.
func clampToCeiling(mode, ceiling string) string {
	if withinCeiling(mode, ceiling) {
		return normalizeACPToolApprovalMode(mode)
	}
	return ceiling
}

func aboveCeilingError(method, mode, ceiling string) *RPCError {
	return &RPCError{
		Code:    ErrInvalidParams,
		Message: method + ": tool approval " + mode + " is above the ceiling this agent was started with (" + ceiling + ")",
		Data:    ToolApprovalData{Code: codeToolApprovalAboveCeiling, Ceiling: ceiling},
	}
}
