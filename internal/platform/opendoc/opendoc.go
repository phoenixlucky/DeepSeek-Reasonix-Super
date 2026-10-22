package opendoc

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNotAbsolute is a path that is empty, relative, or not a plain file path.
var ErrNotAbsolute = errors.New("opendoc: the path must be absolute")

// start launches a program without waiting for it; tests replace it.
var start = realStart

func realStart(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Open asks the platform to open path with its default program.
func Open(path string) error {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || strings.Contains(path, "://") {
		return ErrNotAbsolute
	}
	name, args := command(runtime.GOOS, path)
	return start(name, args...)
}

func command(goos, path string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{path}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", path}
	default:
		return "xdg-open", []string{path}
	}
}
