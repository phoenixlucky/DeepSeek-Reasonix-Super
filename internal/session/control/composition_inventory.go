package control

import (
	"fmt"
	"sort"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/command"
	"reasonix/internal/ext/hook"
	"reasonix/internal/ext/pluginpkg"
	"reasonix/internal/ext/skill"
	"reasonix/internal/state/agentpreset"
)

// choiceMaxRunes bounds a description to what fits in a menu entry.
const choiceMaxRunes = 120

// choiceDescription is the first line of what a module says about itself,
// bounded for a menu. Tool descriptions are written for the model and run to
// paragraphs; the first line is its statement of purpose.
func choiceDescription(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	line = strings.TrimSpace(line)
	if r := []rune(line); len(r) > choiceMaxRunes {
		return strings.TrimSpace(string(r[:choiceMaxRunes])) + "…"
	}
	return line
}

func (c *Controller) compositionInventory() (map[string][]agentpreset.Choice, error) {
	s := c.PresetStore()
	cfg, err := config.RootsForHome(s.Home).LoadForRootReadOnly(s.Workspace)
	if err != nil {
		return nil, err
	}
	out := map[string][]agentpreset.Choice{}
	add := func(kind, id, description, source string) {
		out[kind] = append(out[kind], agentpreset.Choice{ID: id, Description: choiceDescription(description), Source: source})
	}
	for _, kind := range []string{"tools", "skills", "mcp", "extensions", "hooks", "commands"} {
		out[kind] = []agentpreset.Choice{}
	}
	for _, t := range tool.Builtins() {
		add("tools", t.Name(), t.Description(), "builtin")
	}
	for _, name := range []string{"task", "run_skill", "read_only_skill", "use_capability"} {
		add("tools", name, "", "builtin")
	}
	store := skill.New(skill.Options{ProjectRoot: s.Workspace, ReasonixHomeDir: s.Home, CustomPaths: cfg.SkillCustomPaths(), PluginPaths: cfg.PluginPackageSkillOwners(), PluginAgentPaths: cfg.PluginPackageAgentOwners(), ExcludedPaths: cfg.SkillExcludedPaths(), SuppressWarnings: true})
	for _, sk := range store.CompositionSkills() {
		source := "custom"
		if sk.Scope == skill.ScopeBuiltin {
			source = "builtin"
		}
		add("skills", sk.SlashName(), sk.Description, source)
	}
	for _, m := range cfg.EnabledPlugins(s.Workspace, config.DefaultActivationStore()) {
		add("mcp", m.Name, "", "custom")
	}
	installed, _ := pluginpkg.LoadInstalled(s.Home)
	for _, p := range installed {
		add("extensions", p.Installed.Name, "", "custom")
	}
	cmds, err := command.LoadRoots(cfg.Roots().CommandRootsForRoot(s.Workspace)...)
	if err != nil {
		return nil, err
	}
	for _, cmd := range cmds {
		add("commands", cmd.Name, cmd.Description, "custom")
	}
	for i, h := range hook.Load(hook.LoadOptions{ProjectRoot: s.Workspace, ReasonixHomeDir: s.Home}) {
		add("hooks", fmt.Sprintf("%s:%s:%d", h.Scope, h.Event, i), "", "custom")
	}
	for _, items := range out {
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	}
	return out, nil
}
