package skilllock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"reasonix/internal/base/fileutil"
)

const (
	// FileName is the lock's name inside the Reasonix home.
	FileName = "skills.lock"
	version  = 1
)

// Entry is one installed skill and where its content came from.
type Entry struct {
	Name        string    `json:"name"`
	Scope       string    `json:"scope"`
	Source      string    `json:"source"` // an absolute folder, or a URL for a remote source
	Mode        string    `json:"mode"`   // copy | link
	Digest      string    `json:"digest"` // content digest of what was installed
	InstalledAt time.Time `json:"installedAt"`
}

type file struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

// Path is where the lock for a Reasonix home lives.
func Path(reasonixHome string) string { return filepath.Join(reasonixHome, FileName) }

// ErrUnreadable is a lock that exists but cannot be trusted: it does not parse,
// or names a version this build does not know. A missing file is not an error.
var ErrUnreadable = errors.New("skilllock: the lock cannot be read")

func read(path string) (file, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{Version: version}, nil
	}
	if err != nil {
		return file{}, errors.Join(ErrUnreadable, err)
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != version {
		return file{}, errors.Join(ErrUnreadable, err)
	}
	return f, nil
}

// Entries lists what the lock holds. An unreadable lock is reported rather than
// read as empty, so a caller can say "unknown" instead of "nothing recorded".
func Entries(reasonixHome string) ([]Entry, error) {
	f, err := read(Path(reasonixHome))
	if err != nil {
		return nil, err
	}
	return slices.Clone(f.Entries), nil
}

// Find is the entry for one skill, if the lock has one.
func Find(reasonixHome, name, scope string) (Entry, bool, error) {
	entries, err := Entries(reasonixHome)
	if err != nil {
		return Entry{}, false, err
	}
	for _, e := range entries {
		if e.Name == name && e.Scope == scope {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}

// Record adds or replaces the entry for a skill. A lock that cannot be read is
// replaced by one holding just this entry: refusing to write would leave every
// later install unrecorded, and the old content could not be trusted either way.
func Record(reasonixHome string, e Entry) error {
	f, err := read(Path(reasonixHome))
	if err != nil {
		f = file{Version: version}
	}
	f.Entries = slices.DeleteFunc(f.Entries, func(o Entry) bool { return o.Name == e.Name && o.Scope == e.Scope })
	f.Entries = append(f.Entries, e)
	sort.Slice(f.Entries, func(i, j int) bool {
		if f.Entries[i].Scope != f.Entries[j].Scope {
			return f.Entries[i].Scope < f.Entries[j].Scope
		}
		return f.Entries[i].Name < f.Entries[j].Name
	})
	return write(Path(reasonixHome), f)
}

// Forget drops the entry for a skill that was uninstalled.
func Forget(reasonixHome, name, scope string) error {
	f, err := read(Path(reasonixHome))
	if err != nil {
		return nil
	}
	before := len(f.Entries)
	f.Entries = slices.DeleteFunc(f.Entries, func(o Entry) bool { return o.Name == name && o.Scope == scope })
	if len(f.Entries) == before {
		return nil
	}
	return write(Path(reasonixHome), f)
}

func write(path string, f file) error {
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(path, append(raw, '\n'), 0o600)
}

// A skill bigger than these is not digested: the listing reads every recorded
// source on each refresh, so an unbounded folder would stall the Skills list.
// The bytes match what an install copies.
const (
	MaxFiles = 400
	MaxBytes = 20 << 20
)

// ErrTooLarge is a skill folder past MaxFiles or MaxBytes.
var ErrTooLarge = errors.New("skilllock: the skill is too large to digest")

// File is one file of a skill as an install materialises it.
type File struct {
	Rel  string // slash-separated, relative to the skill root
	Path string // where its bytes are read from
	Size int64
	Exec bool
}

// Tree lists a skill folder the way an install copies it: regular files, a link
// that resolves to a regular file inside the folder as that file, and nothing
// else; a .git folder is not part of a skill. A link that escapes the folder,
// is broken or is not a regular file is left out, never followed. Sorted by
// path, and bounded by MaxFiles and MaxBytes.
func Tree(root string) ([]File, error) {
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	var files []File
	var total int64
	err = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		src := p
		if !d.Type().IsRegular() {
			if d.Type()&fs.ModeSymlink == 0 {
				return nil
			}
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				return nil
			}
			if r, err := filepath.Rel(base, resolved); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
				return nil
			}
			src = resolved
		}
		fi, err := os.Stat(src)
		if err != nil || !fi.Mode().IsRegular() {
			return nil
		}
		total += fi.Size()
		if len(files) >= MaxFiles || total > MaxBytes {
			return ErrTooLarge
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		files = append(files, File{Rel: filepath.ToSlash(rel), Path: src, Size: fi.Size(), Exec: fi.Mode().Perm()&0o111 != 0})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}

// FileDigest is the sha256 of one file's bytes.
func FileDigest(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Digest is the content digest of a skill: its Tree by relative path and file
// bytes, or the one file itself. Names and bytes are in it and nothing about
// time or permissions, so a copy digests the same as its source.
func Digest(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	add := func(rel, p string) error {
		sum, err := FileDigest(p)
		if err != nil {
			return err
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write([]byte(sum))
		return nil
	}
	if !info.IsDir() {
		if info.Size() > MaxBytes {
			return "", ErrTooLarge
		}
		if err := add(filepath.Base(path), path); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
	files, err := Tree(path)
	if err != nil {
		return "", err
	}
	for _, f := range files {
		if err := add(f.Rel, f.Path); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// State is how an installed skill stands against its recorded source.
type State string

const (
	Synced        State = "synced"         // neither side has moved since the install
	SourceChanged State = "source_changed" // the source moved on; the installed copy is as installed
	LocalEdited   State = "local_edited"   // the installed copy was changed; the source is as installed
	Diverged      State = "diverged"       // both moved
	SourceMissing State = "source_missing" // the recorded folder is gone
	Unknown       State = "unknown"        // nothing trustworthy to compare: no record, a damaged lock, a remote source or an unreadable side
)

// Assess compares what was installed with what is installed now and with the
// source now. A link-mode skill is its source, so it is synced by construction.
func Assess(e Entry, installedPath string) State {
	if e.Digest == "" || e.Source == "" {
		return Unknown
	}
	if e.Mode == "link" {
		if _, err := os.Stat(e.Source); err != nil {
			return SourceMissing
		}
		return Synced
	}
	if !filepath.IsAbs(e.Source) {
		return Unknown
	}
	if _, err := os.Stat(e.Source); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SourceMissing
		}
		return Unknown
	}
	now, err := Digest(installedPath)
	if err != nil {
		return Unknown
	}
	src, err := Digest(e.Source)
	if err != nil {
		return Unknown
	}
	local, upstream := now != e.Digest, src != e.Digest
	switch {
	case local && upstream:
		return Diverged
	case local:
		return LocalEdited
	case upstream:
		return SourceChanged
	}
	return Synced
}
