//go:build windows

package control

import (
	"os/exec"

	"reasonix/internal/base/proc"
)

// setShellKillTree hides the child's console and makes a cancelled command kill
// its whole process tree, which Windows does not cascade on its own. proc owns
// that kill, including the bound on a taskkill that never answers.
func setShellKillTree(cmd *exec.Cmd) {
	proc.SetCancelKillsTree(cmd)
}
