//go:build windows

package sandbox

// GrantedRoots reads the record under stateDir: this install's SID and the
// folders it is granted on. A missing record is an empty answer, not an error.
func GrantedRoots(stateDir string) (string, []string, error) {
	b, err := OpenGrantBook(stateDir, WindowsGrantOps())
	if err != nil {
		return "", nil, err
	}
	return b.SID(), b.Roots(), nil
}

// RevokeGrants removes the SID from every recorded folder and clears the record.
// It returns the folders revoked and, per folder, why one could not be.
func RevokeGrants(stateDir string) ([]string, map[string]error, error) {
	b, err := OpenGrantBook(stateDir, WindowsGrantOps())
	if err != nil {
		return nil, nil, err
	}
	var done []string
	failed := map[string]error{}
	for _, root := range b.Roots() {
		if err := b.Revoke(root); err != nil {
			failed[root] = err
			continue
		}
		done = append(done, root)
	}
	return done, failed, nil
}
