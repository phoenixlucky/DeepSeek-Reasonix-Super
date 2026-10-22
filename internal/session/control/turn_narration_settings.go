package control

import "reasonix/internal/contract/config"

// TurnNarrationSettings is how much the model says between tool calls: brief or
// detailed. Path is the user config that holds it.
type TurnNarrationSettings struct {
	Level string `json:"level"`
	Path  string `json:"path"`
}

// TurnNarration is the level the next turn carries.
func (c *Controller) TurnNarration() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.display.turnNarration
}

// TurnNarrationSettings reads the stored level, so a window opened elsewhere
// shows what the file says rather than what this session was built with.
func (c *Controller) TurnNarrationSettings() TurnNarrationSettings {
	path := config.UserConfigPath()
	return TurnNarrationSettings{Level: config.LoadForEdit(path).TurnNarration(), Path: path}
}

// SaveTurnNarrationSettings stores the level and applies it to the next turn;
// no rebuild, because the instruction rides the turn and not the stable prompt.
func (c *Controller) SaveTurnNarrationSettings(level string) error {
	mode := config.NormalizeTurnNarration(level)
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	cfg.Agent.TurnNarration = mode
	if err := cfg.SaveTo(path); err != nil {
		return err
	}
	c.mu.Lock()
	c.display.turnNarration = mode
	c.mu.Unlock()
	return nil
}
