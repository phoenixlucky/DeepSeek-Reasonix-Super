package serve

import (
	"path/filepath"

	"reasonix/internal/contract/config"
	"reasonix/internal/ext/skilllock"
)

// skillSource is where an installed skill came from and whether it is still in
// step with that folder. State is a skilllock.State code; the window words it.
type skillSource struct {
	Path  string `json:"path"`
	Mode  string `json:"mode"`
	State string `json:"state"`
}

// skillSources reads the install record once per listing. An unreadable record
// yields nothing: no row claims a state it could not compare.
type skillSources map[string]skilllock.Entry

func loadSkillSources() skillSources {
	entries, err := skilllock.Entries(config.ReasonixHomeDir())
	if err != nil {
		return nil
	}
	out := make(skillSources, len(entries))
	for _, e := range entries {
		if e.Scope == "global" {
			out[e.Name] = e
		}
	}
	return out
}

// of answers for a global skill installed in the canonical <name>/SKILL.md
// layout; anything else was not recorded and has no source to report.
func (s skillSources) of(name, scope, skillPath string) *skillSource {
	e, ok := s[name]
	if !ok || scope != "global" || filepath.Base(skillPath) != "SKILL.md" {
		return nil
	}
	return &skillSource{Path: e.Source, Mode: e.Mode, State: string(skilllock.Assess(e, filepath.Dir(skillPath)))}
}
