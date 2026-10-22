package control

import (
	"context"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/ext/command"
	"reasonix/internal/ext/skill"
)

// Commands returns the loaded custom slash commands.
func (c *Controller) Commands() []command.Command {
	if summary := c.CompositionSummary(); summary != nil && summary.ResourceMode == "live" {
		b := c.CompositionBinding()
		cmds, _ := command.LoadRoots(config.RootsForHome(c.PresetStore().Home).CommandRootsForRoot(c.workspaceRoot)...)
		return compositionCommands(cmds, b.Resolved)
	}
	if p := c.commands.Load(); p != nil {
		return *p
	}
	return nil
}

// ReloadCommands rescans all command directories and hot-swaps the slash_command
// tool and the internal command slice — no MCP restart, no hook rerun.
func (c *Controller) ReloadCommands(ctx context.Context) error {
	if c.CompositionSummary() != nil {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	cmds, loadErr := command.LoadRoots(config.CommandRootsForRoot(c.workspaceRoot)...)
	var cmdSkills []skill.Skill
	if !c.skills.noImplicitInvocation {
		cmdSkills = skill.ModelInvocable(c.SlashSkills())
	}

	entries := make([]command.SlashEntry, 0, len(cmdSkills)+len(cmds))
	for _, sk := range cmdSkills {
		entries = append(entries, command.SlashEntry{
			Name:        sk.SlashName(),
			Description: sk.Description,
			ArgHint:     sk.ArgumentHint,
			Skill:       true,
			Unlisted:    func() bool { return !c.skills.pathHits.Eligible(sk) },
			Render: func(args []string) string {
				cur, err := c.skills.forModel(sk)
				if err != nil {
					return ""
				}
				return c.skills.renderInvocation(cur, strings.Join(args, " "))
			},
		})
	}
	for _, cmd := range cmds {
		if cmd.Hidden {
			continue
		}

		entries = append(entries, command.SlashEntry{
			Name:        cmd.Name,
			Description: cmd.Description,
			ArgHint:     cmd.ArgHint,
			Render:      func(args []string) string { return cmd.Render(args) },
		})
	}
	c.mcp.registerTool(command.NewSlashCommandTool(entries, c.skills.modelGate()))
	cmdSlice := cmds
	c.commands.Store(&cmdSlice)
	return loadErr
}
