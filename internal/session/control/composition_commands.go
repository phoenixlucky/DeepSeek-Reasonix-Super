package control

import (
	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/ext/command"
)

func compositionCommands(commands []command.Command, d agentcomposition.Definition) []command.Command {
	out := []command.Command{}
	for _, cmd := range commands {
		if d.Commands.Allows(cmd.Name) && (cmd.Plugin == "" || d.Extensions.Allows(cmd.Plugin)) {
			out = append(out, cmd)
		}
	}
	return out
}
