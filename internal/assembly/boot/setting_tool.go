package boot

import (
	"slices"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/tool"
	"reasonix/internal/tools/builtin"
)

// addSettingTool offers the model the setting tool unless the user turned it off
// or left it out of an explicit tool list. The link reaches the controller once
// it exists.
func addSettingTool(reg *tool.Registry, cfg *config.Config) *tool.SettingsLink {
	link := &tool.SettingsLink{}
	if !cfg.Tools.SettingToolEnabled() || (len(cfg.Tools.Enabled) > 0 && !slices.Contains(cfg.Tools.Enabled, "setting")) {
		return link
	}
	reg.Add(builtin.SettingTool(link))
	return link
}
