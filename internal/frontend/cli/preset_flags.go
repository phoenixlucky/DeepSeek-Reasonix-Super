package cli

import (
	"fmt"
	"os"
	"strings"

	"reasonix/internal/contract/agentpreset"
)

// livePresets joins the execution settings agentpreset still has. A retired
// name keeps parsing for old scripts but is never offered as a choice.
func livePresets(sep string) string {
	all := agentpreset.All()
	names := make([]string, len(all))
	for i, p := range all {
		names[i] = string(p)
	}
	return strings.Join(names, sep)
}

func presetFlagUsage() string { return "agent execution setting: " + livePresets(" | ") }

func warnProfileDeprecated() {
	fmt.Fprintln(os.Stderr, "warning: --profile is deprecated; use --preset "+livePresets("|"))
}
