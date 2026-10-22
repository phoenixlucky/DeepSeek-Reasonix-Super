//go:build !darwin && !windows

package sandbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// maxBoundDevices bounds how many device nodes a set of globs may bind, so a
// broad pattern cannot turn the sandbox's /dev back into the host's.
const maxBoundDevices = 64

// bwrapDeviceArgs binds the host device nodes the user allowed into the
// sandbox's private /dev. Only existing character or block device nodes under
// /dev qualify: a pattern that leaves /dev, a symlink, a directory and a
// regular file are all skipped, so the list can only ever name devices.
func bwrapDeviceArgs(patterns []string) []string {
	return deviceBindArgs(patterns, filepath.Glob, func(path string) (fs.FileMode, error) {
		info, err := os.Lstat(path)
		if err != nil {
			return 0, err
		}
		return info.Mode(), nil
	})
}

func deviceBindArgs(patterns []string, glob func(string) ([]string, error), mode func(string) (fs.FileMode, error)) []string {
	var nodes []string
	for _, pattern := range patterns {
		clean := filepath.Clean(strings.TrimSpace(pattern))
		if !strings.HasPrefix(clean, "/dev/") || strings.Contains(pattern, "..") {
			continue
		}
		matches, err := glob(clean)
		if err != nil {
			continue
		}
		for _, match := range matches {
			if !strings.HasPrefix(match, "/dev/") || slices.Contains(nodes, match) {
				continue
			}
			m, err := mode(match)
			if err != nil || m&fs.ModeSymlink != 0 || m&fs.ModeDevice == 0 {
				continue
			}
			nodes = append(nodes, match)
		}
	}
	slices.Sort(nodes)
	if len(nodes) > maxBoundDevices {
		nodes = nodes[:maxBoundDevices]
	}
	var args []string
	for _, node := range nodes {
		args = append(args, "--dev-bind", node, node)
	}
	return args
}
