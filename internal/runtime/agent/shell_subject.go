package agent

import (
	"encoding/json"
	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/shellrun"
)

// annotateShellSubject records the part of the command that names what it runs,
// when an assignment prefix stands in front of it.
func annotateShellSubject(execution *tool.ShellExecution, args json.RawMessage) {
	if execution == nil || execution.Subject != "" {
		return
	}
	cmd := bashCommandFromArgs(args)
	if cmd == "" {
		return
	}
	if subject, cut := shellrun.OperativeCommand(cmd); cut {
		execution.Subject = subject
	}
}
