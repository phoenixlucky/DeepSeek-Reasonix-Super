package control

import "reasonix/internal/contract/config"

// FeedbackLevelNoticeSettings is whether the person is told when their feedback
// level goes up. The user file holds it; off by default.
type FeedbackLevelNoticeSettings struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

func (c *Controller) FeedbackLevelNotices() FeedbackLevelNoticeSettings {
	path := config.UserConfigPath()
	return FeedbackLevelNoticeSettings{Enabled: config.LoadForEdit(path).UI.FeedbackLevelNotices, Path: path}
}

// SaveFeedbackLevelNotices needs no rebuild: only what is said depends on it.
func (c *Controller) SaveFeedbackLevelNotices(enabled bool) error {
	unlock := config.LockUserConfigEdits()
	defer unlock()
	path := config.UserConfigPath()
	cfg := config.LoadForEdit(path)
	cfg.UI.FeedbackLevelNotices = enabled
	return cfg.SaveTo(path)
}
