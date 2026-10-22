package memory

import (
	"os"
	"path/filepath"
	"strings"
)

// rebuildIndexIn republishes MEMORY.md from the fact files when the existing
// index could not be read. The unreadable entry is moved aside first so its
// bytes survive; if that rename fails, cause is returned and nothing is
// published. The derived lines are the whole new index.
func (s Store) rebuildIndexIn(dir string, entries []os.DirEntry, cause error) error {
	if err := moveIndexAside(dir); err != nil {
		return cause
	}
	lines := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == indexFile || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		m, ok := loadMemory(filepath.Join(dir, entry.Name()))
		if !ok {
			continue
		}
		m.Name = slug(m.Name)
		if m.Scope == "" {
			m.Scope = s.scopeForDir(dir)
		}
		lines[m.Name] = renderIndexLine(m.Name, m)
	}
	return flushIndexFrom(dir, nil, lines)
}
