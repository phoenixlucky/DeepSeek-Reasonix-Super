//go:build !darwin && !windows

package sandbox

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"
)

func fakeDev(nodes map[string]fs.FileMode) (func(string) ([]string, error), func(string) (fs.FileMode, error)) {
	glob := func(pattern string) ([]string, error) {
		var out []string
		for path := range nodes {
			if ok, _ := matchGlob(pattern, path); ok {
				out = append(out, path)
			}
		}
		return out, nil
	}
	mode := func(path string) (fs.FileMode, error) {
		m, ok := nodes[path]
		if !ok {
			return 0, fs.ErrNotExist
		}
		return m, nil
	}
	return glob, mode
}

func matchGlob(pattern, name string) (bool, error) { return globMatch(pattern, name) }

func TestOnlyExistingDeviceNodesUnderDevAreBound(t *testing.T) {
	glob, mode := fakeDev(map[string]fs.FileMode{
		"/dev/nvidia0":     fs.ModeDevice | fs.ModeCharDevice,
		"/dev/nvidiactl":   fs.ModeDevice | fs.ModeCharDevice,
		"/dev/nvidia-link": fs.ModeSymlink,
		"/dev/nvidia-dir":  fs.ModeDir,
		"/dev/nvidia-file": 0,
	})
	got := deviceBindArgs([]string{"/dev/nvidia*"}, glob, mode)
	want := []string{"--dev-bind", "/dev/nvidia0", "/dev/nvidia0", "--dev-bind", "/dev/nvidiactl", "/dev/nvidiactl"}
	if !slices.Equal(got, want) {
		t.Fatalf("args = %v, want only the two device nodes %v", got, want)
	}
}

func TestADevicePatternThatLeavesDevBindsNothing(t *testing.T) {
	glob, mode := fakeDev(map[string]fs.FileMode{"/dev/nvidia0": fs.ModeDevice, "/etc/passwd": fs.ModeDevice})
	for _, p := range []string{"/etc/*", "nvidia*", "/dev/../etc/passwd", "", "/dev", "/dev/../dev/nvidia0"} {
		if got := deviceBindArgs([]string{p}, glob, mode); len(got) != 0 {
			t.Errorf("pattern %q bound %v", p, got)
		}
	}
}

func TestBoundDevicesAreBoundedAndNotRepeated(t *testing.T) {
	nodes := map[string]fs.FileMode{}
	for i := range maxBoundDevices + 20 {
		nodes[fmt.Sprintf("/dev/gpu%03d", i)] = fs.ModeDevice
	}
	glob, mode := fakeDev(nodes)
	got := deviceBindArgs([]string{"/dev/gpu*", "/dev/gpu0*"}, glob, mode)
	if len(got) != maxBoundDevices*3 {
		t.Fatalf("bound %d arguments, want %d (three per node, capped)", len(got), maxBoundDevices*3)
	}
}

func TestNoDevicesAllowedLeavesThePrivateDevAlone(t *testing.T) {
	if got := bwrapDeviceArgs(nil); len(got) != 0 {
		t.Fatalf("no allow list produced %v", got)
	}
}

func globMatch(pattern, name string) (bool, error) { return filepath.Match(pattern, name) }
