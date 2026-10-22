package skillupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"reasonix/internal/base/diff"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/skill"
	"reasonix/internal/ext/skilllock"
)

// Why an update did not happen, told apart because each asks for something
// different: a different skill, a different install, a re-read, a confirmation
// or a retry.
var (
	ErrBadName       = errors.New("skillupdate: not a skill name")
	ErrNotRecorded   = errors.New("skillupdate: this skill has no recorded source")
	ErrNotLocal      = errors.New("skillupdate: this skill was not copied from a local folder")
	ErrSourceMissing = errors.New("skillupdate: the source folder is gone")
	ErrStale         = errors.New("skillupdate: the plan no longer matches what is on disk")
	ErrNeedsConfirm  = errors.New("skillupdate: the installed copy was edited and overwriting it needs confirmation")
	ErrTooLarge      = errors.New("skillupdate: the skill is too large to update")
	ErrFailed        = errors.New("skillupdate: the update could not be carried out")
)

// Change kinds, as a plan lists them.
const (
	Added   = "added"
	Changed = "changed"
	Removed = "removed"
)

// Limits on the text a plan shows, so a plan stays something a person can read.
const (
	maxDiffFile  = 64 << 10
	maxDiffTotal = 256 << 10
)

// Change is one file an update would touch. Code is any file that is not a
// Markdown document: those are what a person has to read before agreeing.
type Change struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Code      bool   `json:"code"`
	Diff      string `json:"diff,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Binary    bool   `json:"binary,omitempty"`
}

// Plan is what an update would do, as the person is shown it.
type Plan struct {
	Name            string   `json:"name"`
	Source          string   `json:"source"`
	State           string   `json:"state"`
	Changes         []Change `json:"changes"`
	OverwritesLocal bool     `json:"overwritesLocal"`
	PlanID          string   `json:"planId"`
}

// Updater works on one Reasonix home.
type Updater struct{ Home string }

// rename is a variable so a test can make the swap fail part way.
var rename = os.Rename

var (
	recordRetryPause = 150 * time.Millisecond
	recordEntry      = skilllock.Record
)

func (u Updater) installedDir(name string) string {
	return filepath.Join(u.Home, skill.SkillsDirname, name)
}

type reading struct {
	entry       skilllock.Entry
	installed   string
	state       skilllock.State
	src, inst   []skilllock.File
	srcDigest   string
	instDigest  string
	changes     []Change
	changeSigns []string
}

func (u Updater) read(name string) (*reading, error) {
	if !config.IsValidSkillName(name) {
		return nil, ErrBadName
	}
	entry, ok, err := skilllock.Find(u.Home, name, "global")
	if err != nil || !ok {
		return nil, ErrNotRecorded
	}
	if entry.Mode != "copy" || !filepath.IsAbs(entry.Source) {
		return nil, ErrNotLocal
	}
	if info, err := os.Stat(entry.Source); err != nil || !info.IsDir() {
		return nil, ErrSourceMissing
	}
	installed := u.installedDir(name)
	if info, err := os.Lstat(installed); err != nil || !info.IsDir() {
		return nil, ErrNotLocal
	}
	r := &reading{entry: entry, installed: installed}
	if r.src, err = skilllock.Tree(entry.Source); err != nil {
		return nil, tooLargeOr(err)
	}
	if r.inst, err = skilllock.Tree(installed); err != nil {
		return nil, tooLargeOr(err)
	}
	if r.srcDigest, err = skilllock.Digest(entry.Source); err != nil {
		return nil, tooLargeOr(err)
	}
	if r.instDigest, err = skilllock.Digest(installed); err != nil {
		return nil, tooLargeOr(err)
	}
	r.state = skilllock.Assess(entry, installed)
	return r, nil
}

func tooLargeOr(err error) error {
	if errors.Is(err, skilllock.ErrTooLarge) {
		return ErrTooLarge
	}
	return fmt.Errorf("%w: %w", ErrFailed, err)
}

func isCode(rel string) bool {
	lower := strings.ToLower(rel)
	return !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown")
}

// Plan describes what bringing the installed copy up to the source would change.
func (u Updater) Plan(name string) (Plan, error) {
	r, err := u.read(name)
	if err != nil {
		return Plan{}, err
	}
	return r.plan(name)
}

func (r *reading) plan(name string) (Plan, error) {
	if err := r.compare(); err != nil {
		return Plan{}, err
	}
	p := Plan{
		Name: name, Source: r.entry.Source, State: string(r.state), Changes: r.changes,
		OverwritesLocal: r.state == skilllock.LocalEdited || r.state == skilllock.Diverged,
	}
	h := sha256.New()
	for _, part := range []string{name, r.entry.Source, r.entry.Digest, r.srcDigest, r.instDigest, string(r.state)} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	for _, s := range r.changeSigns {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	p.PlanID = hex.EncodeToString(h.Sum(nil))
	return p, nil
}

func (r *reading) compare() error {
	byRel := func(files []skilllock.File) map[string]skilllock.File {
		m := make(map[string]skilllock.File, len(files))
		for _, f := range files {
			m[f.Rel] = f
		}
		return m
	}
	src, inst := byRel(r.src), byRel(r.inst)
	var rels []string
	for rel := range src {
		rels = append(rels, rel)
	}
	for rel := range inst {
		if _, both := src[rel]; !both {
			rels = append(rels, rel)
		}
	}
	sort.Strings(rels)
	budget := maxDiffTotal
	for _, rel := range rels {
		s, hasSrc := src[rel]
		i, hasInst := inst[rel]
		c := Change{Path: rel, Code: isCode(rel)}
		var oldText, newText string
		var sd, id string
		var err error
		switch {
		case hasSrc && !hasInst:
			c.Kind = Added
			if sd, err = skilllock.FileDigest(s.Path); err != nil {
				return fmt.Errorf("%w: %w", ErrFailed, err)
			}
			newText, c.Truncated = readText(s, &budget)
		case !hasSrc && hasInst:
			c.Kind = Removed
			if id, err = skilllock.FileDigest(i.Path); err != nil {
				return fmt.Errorf("%w: %w", ErrFailed, err)
			}
			oldText, c.Truncated = readText(i, &budget)
		default:
			if sd, err = skilllock.FileDigest(s.Path); err != nil {
				return fmt.Errorf("%w: %w", ErrFailed, err)
			}
			if id, err = skilllock.FileDigest(i.Path); err != nil {
				return fmt.Errorf("%w: %w", ErrFailed, err)
			}
			if sd == id {
				continue
			}
			c.Kind = Changed
			oldText, _ = readText(i, &budget)
			newText, c.Truncated = readText(s, &budget)
		}
		if !c.Truncated {
			kind := diff.Modify
			switch c.Kind {
			case Added:
				kind = diff.Create
			case Removed:
				kind = diff.Delete
			}
			built := diff.Build(rel, oldText, newText, kind)
			c.Binary, c.Diff = built.Binary, built.Diff
		}
		r.changes = append(r.changes, c)
		r.changeSigns = append(r.changeSigns, c.Kind+" "+rel+" "+sd+" "+id)
	}
	return nil
}

// readText reads a file for display, within what is left of the plan's budget.
// A file past either bound is reported truncated instead of shown cut.
func readText(f skilllock.File, budget *int) (string, bool) {
	if f.Size > maxDiffFile || int(f.Size) > *budget {
		return "", true
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return "", true
	}
	*budget -= len(b)
	return string(b), false
}

// Apply carries out the plan the person saw. The reading is repeated, and
// anything that moved since (the source, the installed copy, the recorded
// source) makes the plan stale rather than applying something unreviewed.
func (u Updater) Apply(name, planID string, overwriteLocal bool) error {
	r, err := u.read(name)
	if err != nil {
		return err
	}
	current, err := r.plan(name)
	if err != nil {
		return err
	}
	if current.PlanID != planID {
		return ErrStale
	}
	if current.OverwritesLocal && !overwriteLocal {
		return ErrNeedsConfirm
	}
	if len(current.Changes) == 0 {
		return u.healRecord(r)
	}
	return u.swap(r)
}

func (u Updater) swap(r *reading) error {
	work := filepath.Join(u.Home, ".skill-update")
	if err := os.MkdirAll(work, 0o700); err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	stage, err := os.MkdirTemp(work, "new-")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	defer os.RemoveAll(stage)
	for _, f := range r.src {
		if err := materialise(f, filepath.Join(stage, filepath.FromSlash(f.Rel))); err != nil {
			return fmt.Errorf("%w: %w", ErrFailed, err)
		}
	}
	if got, err := skilllock.Digest(stage); err != nil || got != r.srcDigest {
		return ErrStale
	}
	aside := filepath.Join(work, "old-"+filepath.Base(stage))
	if err := rename(r.installed, aside); err != nil {
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	if err := rename(stage, r.installed); err != nil {
		if back := rename(aside, r.installed); back != nil {
			return fmt.Errorf("%w: %w (and the old copy is kept at %s: %w)", ErrFailed, err, aside, back)
		}
		return fmt.Errorf("%w: %w", ErrFailed, err)
	}
	_ = os.RemoveAll(aside)
	return u.record(r)
}

// record notes the new digest, once more after a short pause: the copy is
// already in place, so a record left behind would make it read as edited.
func (u Updater) record(r *reading) error {
	entry := r.entry
	entry.Digest, entry.InstalledAt = r.srcDigest, time.Now().UTC()
	err := recordEntry(u.Home, entry)
	if err != nil {
		time.Sleep(recordRetryPause)
		err = recordEntry(u.Home, entry)
	}
	if err != nil {
		return fmt.Errorf("%w: the copy is updated, but the record of it could not be written: %w", ErrFailed, err)
	}
	return nil
}

// healRecord is an Apply with nothing to change: when the copy already equals
// the source but the record still holds an older digest (an update whose record
// write failed), it brings the record up to date and the skill reads as in step.
func (u Updater) healRecord(r *reading) error {
	if r.instDigest != r.srcDigest || r.entry.Digest == r.srcDigest {
		return nil
	}
	return u.record(r)
}

// materialise writes one file the way an install does: new, with ordinary
// permissions and the executable bit kept.
func materialise(f skilllock.File, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(f.Path)
	if err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if f.Exec {
		mode = 0o755
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := out.Write(b); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
