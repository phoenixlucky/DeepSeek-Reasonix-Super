package control

import "reasonix/internal/contract/config"

// DefaultModeSettings is whether a new conversation starts in plan mode, as the
// user file records it. It is the user's alone: a project file cannot set it, so
// there is no second effective value.
type DefaultModeSettings struct {
	Plan bool   `json:"plan"`
	Path string `json:"path"`
}

// DefaultModeSettings reads [agent] default_mode from the user file.
func (c *Controller) DefaultModeSettings() DefaultModeSettings {
	path := config.UserConfigPath()
	return DefaultModeSettings{Plan: config.LoadForEdit(path).DefaultPlanMode(), Path: path}
}

// SaveDefaultModeSettings persists the choice to the user file; the next new
// conversation reads it.
func (c *Controller) SaveDefaultModeSettings(plan bool) error {
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	mode := "normal"
	if plan {
		mode = "plan"
	}
	if err := cfg.SetDefaultMode(mode); err != nil {
		return err
	}
	return cfg.SaveTo(path)
}
