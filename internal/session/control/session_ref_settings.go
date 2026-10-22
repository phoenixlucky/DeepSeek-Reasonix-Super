package control

import "reasonix/internal/contract/config"

// SessionRefSettings is the user's limit on what one referenced past
// conversation may add to a message, with the bounds a value must lie in.
type SessionRefSettings struct {
	Tokens        int    `json:"tokens"`
	DefaultTokens int    `json:"defaultTokens"`
	MinTokens     int    `json:"minTokens"`
	MaxTokens     int    `json:"maxTokens"`
	Path          string `json:"path"`
}

func (c *Controller) SessionRefSettings() SessionRefSettings {
	path := config.UserConfigPath()
	return SessionRefSettings{
		Tokens:        config.LoadForEdit(path).SessionRefTokens(),
		DefaultTokens: config.DefaultSessionRefTokens,
		MinTokens:     config.SessionRefTokensMin,
		MaxTokens:     config.SessionRefTokensMax,
		Path:          path,
	}
}

// SaveSessionRefSettings persists the limit; each reference reads it as it is
// resolved, so nothing is rebuilt. 0 returns to the default.
func (c *Controller) SaveSessionRefSettings(tokens int) error {
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	if err := cfg.SetSessionRefTokens(tokens); err != nil {
		return err
	}
	return cfg.SaveTo(path)
}
