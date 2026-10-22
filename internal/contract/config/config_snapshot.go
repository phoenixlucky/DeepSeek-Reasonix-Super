package config

import (
	"path/filepath"
	"strings"

	fileencoding "reasonix/internal/base/fileutil/encoding"
)

// ConfigSnapshot is one unlocked read of a config file, shared by the startup
// migrations. It only ever settles that there is nothing to rewrite: a
// migration that does rewrite takes the file's edit lock and reads it again, so
// a stale snapshot can cost a migration until the next start, never an edit.
type ConfigSnapshot struct {
	path   string
	text   string
	exists bool
	err    error
}

// SnapshotUserConfig reads the user config LoadForRoot would read.
func (r Roots) SnapshotUserConfig() *ConfigSnapshot {
	return readConfigSnapshot(r.userConfigLoadPath(), fileencoding.ReadFileUTF8)
}

func readConfigSnapshot(path string, read func(string) ([]byte, error)) *ConfigSnapshot {
	s := &ConfigSnapshot{path: path}
	if strings.TrimSpace(path) == "" {
		return s
	}
	resolved, exists, err := statConfigPath(path)
	if err != nil {
		s.err = err
		return s
	}
	if !exists {
		return s
	}
	raw, err := read(resolved)
	if err != nil {
		s.err = err
		return s
	}
	s.text, s.exists = string(raw), true
	return s
}

// settled reports whether the file needs nothing: it is absent, or untouched
// says so of its text. A file that could not be read is never settled, so the
// locked path meets the same error it always did.
func (s *ConfigSnapshot) settled(untouched func(text string) bool) bool {
	if s.err != nil {
		return false
	}
	return !s.exists || untouched(s.text)
}

func (s *ConfigSnapshot) of(path string) bool {
	return s != nil && s.path != "" && filepath.Clean(s.path) == filepath.Clean(path)
}

// MigrateLegacyDeepSeekProtocolUserConfigFrom is the same upgrade deciding from
// a snapshot already read: a config with nothing to upgrade is neither locked
// nor read again.
func (r Roots) MigrateLegacyDeepSeekProtocolUserConfigFrom(snap *ConfigSnapshot) (bool, error) {
	if strings.TrimSpace(snap.path) == "" {
		return false, nil
	}
	if snap.settled(func(text string) bool {
		_, changed, err := rewriteLegacyDeepSeekProtocol(text, "", true)
		return err == nil && !changed
	}) {
		return false, nil
	}
	return editLegacyDeepSeekProtocolFile(snap.path, "", true)
}
