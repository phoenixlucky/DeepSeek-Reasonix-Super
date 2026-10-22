package sandbox

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"reasonix/internal/base/fileutil"
)

// grantOps is the Windows ACL work the grant book decides when to do. The real
// implementation edits a directory's DACL; tests supply one that records.
type grantOps interface {
	Grant(root, sid string) error
	Revoke(root, sid string) error
	Granted(root, sid string) (bool, error)
}

// Grant states are written before the ACL is touched, so a crash between the
// intent and the edit leaves a record that says what to finish.
const (
	grantGranting = "granting"
	grantGranted  = "granted"
	grantRevoking = "revoking"
)

const grantBookVersion = 1

const grantBookFile = "windows-grants.json"

// ErrGrantBookUnreadable is a record that exists and cannot be trusted. The
// sandbox treats it as "no grants held" and refuses to run, never as empty.
var ErrGrantBookUnreadable = errors.New("sandbox: the Windows grant record is unreadable")

type grantEntry struct {
	Root  string `json:"root"`
	State string `json:"state"`
}

type grantFile struct {
	Version int          `json:"version"`
	SID     string       `json:"sid"`
	Roots   []grantEntry `json:"roots"`
	Updated time.Time    `json:"updated"`
}

// GrantBook records which directories carry this install's synthetic SID. A
// leaked entry is inert (no token carries the SID), so the book exists to repair
// and revoke, not to keep a hole closed.
type GrantBook struct {
	mu   sync.Mutex
	path string
	ops  grantOps
	file grantFile
}

// GrantRepair says what Repair did, root by root.
type GrantRepair struct {
	Regranted, Revoked, Dropped []string
	Failed                      map[string]error
}

// newSyntheticSID makes a SID in the machine-domain range with random
// sub-authorities: it names no account, and no token carries it unless the
// launcher puts it there.
func newSyntheticSID() string {
	for {
		var b [16]byte
		_, _ = rand.Read(b[:])
		sub := func(i int) uint32 { return binary.BigEndian.Uint32(b[i*4 : i*4+4]) }
		sid := fmt.Sprintf("S-1-5-21-%d-%d-%d-%d", sub(0), sub(1), sub(2), 10000+sub(3)%50000)
		if !sidOwned(sid) {
			return sid
		}
	}
}

// syntheticSID accepts only a SID this package could have made and that names no
// account: a domain user's SID has the same shape, and a record naming one would
// grant, and later revoke, that account's access.
func syntheticSID(s string) bool {
	return syntheticShape(s) && !sidOwned(s)
}

// sidOwned is namesAnAccount; tests replace it to stand in for a domain account.
var sidOwned = namesAnAccount

func syntheticShape(s string) bool {
	var a, b, c, rid uint32
	var tail string
	n, _ := fmt.Sscanf(s, "S-1-5-21-%d-%d-%d-%d%s", &a, &b, &c, &rid, &tail)
	return n == 4 && rid >= 10000 && rid < 60000 && s == fmt.Sprintf("S-1-5-21-%d-%d-%d-%d", a, b, c, rid)
}

// OpenGrantBook loads the record under dir, or starts one with a fresh SID. A
// record that is present and unreadable is an error, not a new book.
func OpenGrantBook(dir string, ops grantOps) (*GrantBook, error) {
	b := &GrantBook{path: filepath.Join(dir, grantBookFile), ops: ops}
	data, err := os.ReadFile(b.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		b.file = grantFile{Version: grantBookVersion, SID: newSyntheticSID()}
		return b, b.saveLocked()
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrGrantBookUnreadable, err)
	}
	if json.Unmarshal(data, &b.file) != nil || b.file.Version != grantBookVersion || !syntheticSID(b.file.SID) {
		return nil, ErrGrantBookUnreadable
	}
	return b, nil
}

// SID is the synthetic SID this install's grants carry.
func (b *GrantBook) SID() string { return b.file.SID }

func (b *GrantBook) saveLocked() error {
	b.file.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(b.file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o700); err != nil {
		return err
	}
	return fileutil.AtomicWriteFileStrict(b.path, data, 0o600)
}

func grantKey(root string) string { return strings.ToLower(filepath.Clean(root)) }

func (b *GrantBook) index(root string) int {
	key := grantKey(root)
	return slices.IndexFunc(b.file.Roots, func(e grantEntry) bool { return grantKey(e.Root) == key })
}

// Roots lists the directories recorded as granted, in the order they were added.
func (b *GrantBook) Roots() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, e := range b.file.Roots {
		if e.State == grantGranted {
			out = append(out, e.Root)
		}
	}
	return out
}

// Ensure grants the SID on root unless the book already holds it and the
// directory still shows it. The intent is written before the edit and the
// result after, so a crash in between is something Repair finishes.
func (b *GrantBook) Ensure(root string) error {
	root = filepath.Clean(root)
	b.mu.Lock()
	defer b.mu.Unlock()
	if i := b.index(root); i >= 0 && b.file.Roots[i].State == grantGranted {
		if ok, err := b.ops.Granted(root, b.file.SID); err == nil && ok {
			return nil
		}
	}
	return b.setLocked(root, grantGranting, func() error { return b.ops.Grant(root, b.file.SID) }, true)
}

// Revoke removes the SID from root and the root from the book.
func (b *GrantBook) Revoke(root string) error {
	root = filepath.Clean(root)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.index(root) < 0 {
		return nil
	}
	return b.setLocked(root, grantRevoking, func() error { return b.ops.Revoke(root, b.file.SID) }, false)
}

// setLocked writes the intent, runs the edit, then writes the result: a granted
// entry for a grant, no entry for a revoke.
func (b *GrantBook) setLocked(root, intent string, edit func() error, keep bool) error {
	if i := b.index(root); i >= 0 {
		b.file.Roots[i].State = intent
	} else {
		b.file.Roots = append(b.file.Roots, grantEntry{Root: root, State: intent})
	}
	if err := b.saveLocked(); err != nil {
		return err
	}
	if err := edit(); err != nil {
		return err
	}
	i := b.index(root)
	if keep {
		b.file.Roots[i].State = grantGranted
	} else {
		b.file.Roots = slices.Delete(b.file.Roots, i, i+1)
	}
	return b.saveLocked()
}

// Repair finishes what a crash left and checks what the record claims: a
// half-done grant or revoke is completed, a granted root that lost the SID is
// granted again, and a root that no longer exists is dropped from the book.
func (b *GrantBook) Repair() GrantRepair {
	b.mu.Lock()
	defer b.mu.Unlock()
	rep := GrantRepair{Failed: map[string]error{}}
	for _, e := range slices.Clone(b.file.Roots) {
		if _, err := os.Stat(e.Root); errors.Is(err, os.ErrNotExist) {
			b.dropLocked(e.Root)
			rep.Dropped = append(rep.Dropped, e.Root)
			continue
		}
		switch e.State {
		case grantRevoking:
			if err := b.ops.Revoke(e.Root, b.file.SID); err != nil {
				rep.Failed[e.Root] = err
				continue
			}
			b.dropLocked(e.Root)
			rep.Revoked = append(rep.Revoked, e.Root)
		default:
			if ok, err := b.ops.Granted(e.Root, b.file.SID); err == nil && ok && e.State == grantGranted {
				continue
			}
			if err := b.ops.Grant(e.Root, b.file.SID); err != nil {
				rep.Failed[e.Root] = err
				continue
			}
			b.file.Roots[b.index(e.Root)].State = grantGranted
			rep.Regranted = append(rep.Regranted, e.Root)
		}
	}
	if err := b.saveLocked(); err != nil {
		rep.Failed[b.path] = err
	}
	return rep
}

func (b *GrantBook) dropLocked(root string) {
	if i := b.index(root); i >= 0 {
		b.file.Roots = slices.Delete(b.file.Roots, i, i+1)
	}
}
