package installsource

import (
	"os"
	"path/filepath"
	"time"

	"reasonix/internal/ext/skilllock"
)

// recordSkillSource remembers where a global skill's content came from, so the
// installed copy can later be told apart from its source. A project skill is
// not recorded: it lives in the repository, which already says where it is
// from. A failed write is not an install failure; the skill then reads as
// "unknown" against its source, never as in step.
func (t *Tool) recordSkillSource(act *action) {
	if act.Scope != "global" || t.reasonixHome == "" || act.skill.Name == "" {
		return
	}
	source, installed := act.resolvedSource, act.CanonicalPath
	if act.Action == "link_skill" {
		if resolved, err := filepath.EvalSymlinks(act.skill.SourcePath); err == nil {
			source, installed = resolved, resolved
		}
	} else if act.skill.IsDir {
		installed = filepath.Dir(act.CanonicalPath)
	}
	if source == "" {
		source = act.Source
	}
	digest, err := skilllock.Digest(installed)
	if err != nil {
		return
	}
	mode := "copy"
	if act.Action == "link_skill" {
		mode = "link"
	}
	if _, err := os.Stat(installed); err != nil {
		return
	}
	_ = skilllock.Record(t.reasonixHome, skilllock.Entry{
		Name: act.skill.Name, Scope: act.Scope, Source: source, Mode: mode, Digest: digest, InstalledAt: time.Now().UTC(),
	})
}
