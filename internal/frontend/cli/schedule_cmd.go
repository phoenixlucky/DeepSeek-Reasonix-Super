package cli

import (
	"fmt"
	"os"

	"reasonix/internal/base/i18n"
	"reasonix/internal/frontend/schedulerun"
)

// readsLanguageFromConfig says whether the command may read configuration for
// the UI language before it runs. The scheduled-run child reads no file of the
// current directory at all.
func readsLanguageFromConfig(cmd string, doctorRepair bool) bool {
	return !doctorRepair && cmd != "schedule"
}

// unknownCommand is the fallthrough of the command switch: the internal commands
// that are not listed in usage, then the usage error.
func unknownCommand(cmd string, rest []string) int {
	if cmd == "schedule" {
		return scheduleCommand(rest)
	}
	if cmd == "sandbox" {
		return sandboxCommand(rest)
	}
	fmt.Fprintf(os.Stderr, i18n.M.UnknownCommandFmt+"\n\n", cmd)
	usage()
	return 2
}

// scheduleCommand dispatches `reasonix schedule`. exec is the supervisor's own
// child entry, not a command a person types; the rest manage and tick schedules.
func scheduleCommand(args []string) int {
	if len(args) > 0 && args[0] == "exec" {
		return schedulerun.Exec(args[1:])
	}
	return scheduleManage(args, scheduleOS())
}
