package peermail

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"reasonix/internal/base/fileutil"
)

// Dir is the peers directory under the state root.
type Dir struct{ root string }

// Open makes the directory private to this user if it is not there, and
// refuses one that is a link, belongs to someone else, or can be written by
// others: a spool another account can write is not one a sender can be trusted
// through.
func Open(root string) (*Dir, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("peermail: no state directory")
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	for _, sub := range []string{"", "live", "spool", "seen"} {
		if err := checkPrivate(filepath.Join(root, sub), sub != ""); err != nil {
			return nil, err
		}
	}
	return &Dir{root: root}, nil
}

func checkPrivate(path string, create bool) error {
	if create {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !ownedByUser(info) || !privateMode(info) {
		return fmt.Errorf("%w: %s", ErrUnsafeDir, path)
	}
	return nil
}

func (d *Dir) spoolDir(sessionID string) string { return filepath.Join(d.root, "spool", sessionID) }

// PublishLive writes this runtime's live record, replacing its previous one.
func (d *Dir) PublishLive(l Live) error {
	if !l.validate() {
		return ErrInvalid
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFileStrict(filepath.Join(d.root, "live", l.SessionID+".json"), data, 0o600)
}

// RemoveLive withdraws a runtime's record on a clean close.
func (d *Dir) RemoveLive(sessionID string) error {
	if !validID(sessionID) {
		return ErrInvalid
	}
	err := os.Remove(filepath.Join(d.root, "live", sessionID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// RemoveLiveIf withdraws the record only while it still belongs to instance: a
// session rebuilt in place is published by its replacement before the old
// controller closes, and the old one must not take that record down with it.
func (d *Dir) RemoveLiveIf(sessionID, instance string) error {
	if !validID(sessionID) {
		return ErrInvalid
	}
	path := filepath.Join(d.root, "live", sessionID+".json")
	var l Live
	if reason := d.readJSON(path, maxLiveBytes, &l); reason != "" {
		return nil
	}
	if l.Instance != instance {
		return nil
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ListLive returns the records whose runtime the caller's check says is alive.
// A record whose file name is not its own session id is skipped as forged: the
// name is what a sender addressed, so the two must agree.
func (d *Dir) ListLive(alive func(Live) bool) ([]Live, []Rejected) {
	entries, _ := os.ReadDir(filepath.Join(d.root, "live"))
	var out []Live
	var rejected []Rejected
	for _, e := range entries {
		name := e.Name()
		stem, ok := strings.CutSuffix(name, ".json")
		if !ok || strings.HasPrefix(name, ".") {
			continue
		}
		var l Live
		if reason := d.readJSON(filepath.Join(d.root, "live", name), maxLiveBytes, &l); reason != "" {
			rejected = append(rejected, Rejected{name, reason})
			continue
		}
		if l.SessionID != stem {
			rejected = append(rejected, Rejected{name, ReasonNameMismatch})
			continue
		}
		if !l.validate() {
			rejected = append(rejected, Rejected{name, ReasonInvalid})
			continue
		}
		if alive != nil && alive(l) {
			out = append(out, l)
		}
	}
	return out, rejected
}

// Send puts one message into the receiver's spool. The file is written under a
// hidden name, synced, and linked into place, so a reader sees all of it or none
// of it, and an id already spooled is refused rather than replaced.
func (d *Dir) Send(m Message) error {
	m.Body = cleanBody(m.Body)
	if err := m.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	dir := d.spoolDir(m.To.SessionID)
	if err := checkPrivate(dir, true); err != nil {
		return err
	}
	tmp, err := os.OpenFile(filepath.Join(dir, ".tmp-"+m.ID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrDuplicate
		}
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpPath, filepath.Join(dir, m.ID+".json")); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrDuplicate
		}
		return err
	}
	return nil
}

// SweepTemp removes hidden temporary files in sessionID's spool that are older
// than olderThan. A sender that crashed between creating one and linking it
// into place leaves it behind, and until it goes a retry under the same id is
// refused as a duplicate.
func (d *Dir) SweepTemp(sessionID string, olderThan time.Duration) {
	if !validID(sessionID) {
		return
	}
	entries, _ := os.ReadDir(d.spoolDir(sessionID))
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".tmp-") {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || time.Since(info.ModTime()) < olderThan {
			continue
		}
		_ = os.Remove(filepath.Join(d.spoolDir(sessionID), e.Name()))
	}
}

// Pending reads the messages addressed to sessionID, oldest name first. A file
// that is not a message addressed here under its own id is reported and left.
func (d *Dir) Pending(sessionID string) ([]Message, []Rejected) {
	if !validID(sessionID) {
		return nil, nil
	}
	entries, _ := os.ReadDir(d.spoolDir(sessionID))
	var out []Message
	var rejected []Rejected
	for _, e := range entries {
		name := e.Name()
		stem, ok := strings.CutSuffix(name, ".json")
		if !ok || strings.HasPrefix(name, ".") {
			continue
		}
		var m Message
		if reason := d.readJSON(filepath.Join(d.spoolDir(sessionID), name), maxFileBytes, &m); reason != "" {
			rejected = append(rejected, Rejected{name, reason})
			continue
		}
		if m.ID != stem || m.To.SessionID != sessionID {
			rejected = append(rejected, Rejected{name, ReasonNameMismatch})
			continue
		}
		if m.Validate() != nil {
			rejected = append(rejected, Rejected{name, ReasonInvalid})
			continue
		}
		// Send cleans a body, but a file is whatever a process of this user wrote.
		m.Body = cleanBody(m.Body)
		out = append(out, m)
	}
	return out, rejected
}

// readJSON reads one regular file under a size bound; a link or a device is
// never followed.
func (d *Dir) readJSON(path string, limit int64, into any) Reason {
	info, err := os.Lstat(path)
	if err != nil {
		return ReasonUnreadable
	}
	if !info.Mode().IsRegular() {
		return ReasonNotRegular
	}
	if info.Size() > limit {
		return ReasonTooLarge
	}
	f, err := os.Open(path)
	if err != nil {
		return ReasonUnreadable
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return ReasonUnreadable
	}
	if json.Unmarshal(data, into) != nil {
		return ReasonMalformed
	}
	return ""
}

// Seen reports that sessionID already took this message in, which is how a
// message left in the spool by a crash is not delivered twice.
func (d *Dir) Seen(sessionID, id string) bool {
	return slices.Contains(d.seenIDs(sessionID), id)
}

// Ack records the message as taken in and removes it from the spool, in that
// order: a crash between the two leaves a file that is skipped, never one that
// is delivered again.
func (d *Dir) Ack(sessionID, id string) error {
	if !validID(sessionID) || !validID(id) {
		return ErrInvalid
	}
	ids := append(d.seenIDs(sessionID), id)
	if len(ids) > seenKeep {
		ids = ids[len(ids)-seenKeep:]
	}
	if err := fileutil.AtomicWriteFileStrict(filepath.Join(d.root, "seen", sessionID+".log"), []byte(strings.Join(ids, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(d.spoolDir(sessionID), id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (d *Dir) seenIDs(sessionID string) []string {
	if !validID(sessionID) {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(d.root, "seen", sessionID+".log"))
	if err != nil {
		return nil
	}
	var ids []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if id := strings.TrimSpace(sc.Text()); validID(id) {
			ids = append(ids, id)
		}
	}
	return ids
}
