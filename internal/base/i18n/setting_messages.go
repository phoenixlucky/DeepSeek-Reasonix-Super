package i18n

// SettingMessages is how the host words settings to the person: a model's
// request to change one (built from these and the setting's declaration, never
// from text the model wrote), and why a model's reasoning levels cannot be set.
// Messages embeds it.
type SettingMessages struct {
	SettingApprovalFmt     string // %[1]s id, %[2]s old value, %[3]s new value, %[4]s when it applies
	SettingReasonFmt       string // %s the model's own reason, quoted after the card text
	SettingChangedFmt      string // %[1]s id, %[2]s old value, %[3]s new value, %[4]s when it applies
	SettingOpenFmt         string // %s id
	SettingEffectImmediate string
	SettingEffectNextTurn  string
	SettingEffectOnOpen    string
	SettingEffectRestart   string
	EffortUndeclaredFmt    string // %s provider entry name
}
