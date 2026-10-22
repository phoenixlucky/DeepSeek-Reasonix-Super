package control

import (
	"time"

	"reasonix/internal/contract/config"
)

// WriteLeaseSettings is how this machine's sessions hold the workspace write
// lease against one another, as the user file records it. It is the user's
// alone: a project file cannot set it, so there is no second effective value.
type WriteLeaseSettings struct {
	Optimistic bool `json:"optimistic"`
	// MaxWaitMinutes is how long a write waits behind another session's claim
	// before it fails; 0 waits as long as the turn lasts.
	MaxWaitMinutes int    `json:"maxWaitMinutes"`
	Path           string `json:"path"`
}

// WriteLeaseSettings reads [agent] write_lease from the user file.
func (c *Controller) WriteLeaseSettings() WriteLeaseSettings {
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	return WriteLeaseSettings{Optimistic: cfg.WriteLeaseOptimistic(), MaxWaitMinutes: int(cfg.WriteLeaseMaxWait() / time.Minute), Path: path}
}

// SaveWriteLeaseSettings persists the choices given, nil leaving one as it is,
// to the user file. A session keeps the lease it was built with, and a rebuild
// adopts it, so a choice reaches the sessions opened after it, not the ones
// already running.
func (c *Controller) SaveWriteLeaseSettings(optimistic *bool, maxWaitMinutes *int) error {
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	if optimistic != nil {
		cfg.SetWriteLease(*optimistic)
	}
	if maxWaitMinutes != nil {
		cfg.SetWriteLeaseMaxWaitMinutes(*maxWaitMinutes)
	}
	return cfg.SaveTo(path)
}
