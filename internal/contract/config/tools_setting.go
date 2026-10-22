package config

// SettingToolEnabled reports whether the model is offered the setting tool.
// Unset means on: the tool only asks, and nothing changes without the user's answer.
func (t ToolsConfig) SettingToolEnabled() bool {
	return t.SettingTool == nil || *t.SettingTool
}
