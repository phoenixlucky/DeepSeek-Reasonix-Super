package config

// UpdateUserPeers applies change to the [peers] table of the user file and
// saves it. Nothing here touches a project file: the table is user file only,
// so a repository cannot flip it, and a settings surface reaches it only
// through this one writer.
func UpdateUserPeers(change func(*PeersConfig) error) (PeersConfig, error) {
	path := UserConfigPath()
	edit := LoadForEdit(path)
	next := clonePeers(edit.Peers)
	if err := change(&next); err != nil {
		return clonePeers(edit.Peers), err
	}
	edit.Peers = next
	if err := edit.SaveTo(path); err != nil {
		return clonePeers(edit.Peers), err
	}
	return clonePeers(next), nil
}

// UserPeers is the [peers] table as the user file holds it now.
func UserPeers() PeersConfig {
	return clonePeers(LoadForEdit(UserConfigPath()).Peers)
}
