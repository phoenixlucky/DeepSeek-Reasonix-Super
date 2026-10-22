package agent

import (
	"os"

	"reasonix/internal/contract/tool"
)

// rootMissingOutcome refuses a call whose directories no longer exist, before it
// runs. The tool states which directories it acts under; the host is the one that
// knows one is gone, so the cause reaches the model and the frontends as a typed
// refusal rather than as whatever the operating system said, or as an empty
// result that reads as "no matches".
func rootMissingOutcome(t tool.Tool, args []byte) (toolOutcome, bool) {
	err := tool.MissingRoot(tool.RootsOf(t, args), os.Stat)
	if err == nil {
		return toolOutcome{}, false
	}
	return toolOutcome{
		output:      err.Error(),
		blocked:     true,
		errMsg:      firstLine(err.Error()),
		refusalCode: refusalCodeOf(err),
	}, true
}
