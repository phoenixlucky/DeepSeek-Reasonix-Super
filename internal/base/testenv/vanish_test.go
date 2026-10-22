package testenv

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

var errRemovalRace = errors.New("fake removal race")

func TestVanishedMidWalkClassifiesByIdentityAndRestat(t *testing.T) {
	old := removalRaceErrors
	removalRaceErrors = append(append([]error(nil), old...), errRemovalRace)
	t.Cleanup(func() { removalRaceErrors = old })

	gone := filepath.Join(t.TempDir(), "gone")
	present := t.TempDir()
	race := &fs.PathError{Op: "open", Path: "x", Err: errRemovalRace}

	if !VanishedMidWalk(gone, fs.ErrNotExist) {
		t.Error("not-exist must count as vanished")
	}
	if !VanishedMidWalk(gone, race) {
		t.Error("a removal-race error on a path that is gone must count as vanished")
	}
	if VanishedMidWalk(present, race) {
		t.Error("a removal-race error on a path that still exists must fail the walk")
	}
	if VanishedMidWalk(gone, errors.New("other")) {
		t.Error("an unrelated error must fail the walk")
	}
	if VanishedMidWalk(gone, fs.ErrPermission) {
		t.Error("a bare permission error must fail the walk")
	}
}
