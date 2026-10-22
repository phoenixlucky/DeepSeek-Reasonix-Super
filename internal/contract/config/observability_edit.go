package config

// UpdateUserOTLP applies change to the [observability.otlp] table of the user
// file and saves it. A change that returns an error saves nothing.
func UpdateUserOTLP(change func(*OTLPConfig) error) (OTLPConfig, error) {
	unlock := LockUserConfigEdits()
	defer unlock()
	path := UserConfigPath()
	edit := LoadForEdit(path)
	next := edit.Observability.OTLP
	if err := change(&next); err != nil {
		return edit.Observability.OTLP, err
	}
	edit.Observability.OTLP = next
	if err := edit.SaveTo(path); err != nil {
		return edit.Observability.OTLP, err
	}
	return next, nil
}

// UserOTLP is the [observability.otlp] table as the user file holds it now.
func UserOTLP() OTLPConfig { return LoadForEdit(UserConfigPath()).Observability.OTLP }
