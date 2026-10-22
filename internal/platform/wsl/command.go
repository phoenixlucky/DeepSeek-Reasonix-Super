package wsl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"reasonix/internal/base/proc"
)

// maxCapture bounds what one command may hand back on each stream. A file the
// caller wants whole is read in pieces by the layer above, not by raising it.
const maxCapture = 8 << 20

// Result is what a command in a distribution wrote and how it ended. A command
// that ran and failed is a Result with a non-zero ExitCode, not an error: the
// error return is for a command that could not be run at all.
type Result struct {
	Stdout, Stderr []byte
	ExitCode       int
}

// Command runs wsl.exe with args, feeding stdin when it is not nil.
type Command func(ctx context.Context, stdin io.Reader, args ...string) (Result, error)

// SystemCommand runs the wsl.exe in System32, so a PATH entry cannot stand in for it.
func SystemCommand() Command {
	return func(ctx context.Context, stdin io.Reader, args ...string) (Result, error) {
		dir := systemDirectory()
		if runtime.GOOS != "windows" || dir == "" {
			return Result{}, ErrUnavailable
		}
		exe := filepath.Join(dir, "wsl.exe")
		if _, err := os.Stat(exe); err != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		cmd := exec.CommandContext(ctx, exe, args...)
		proc.HideWindow(cmd)
		var out, errb bytes.Buffer
		cmd.Stdin = stdin
		cmd.Stdout = &limited{w: &out, left: maxCapture}
		cmd.Stderr = &limited{w: &errb, left: maxCapture}
		err := cmd.Run()
		res := Result{Stdout: out.Bytes(), Stderr: errb.Bytes()}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, err
	}
}

// ExecArgs is the wsl.exe argument list that runs argv inside a distribution
// with no shell between wsl.exe and the program. The distribution name is its
// own argument, so it cannot become an option.
func ExecArgs(distro string, argv ...string) []string {
	return append([]string{"-d", distro, "--exec"}, argv...)
}

// IsWindowsPath reports whether p is spelled as an absolute Windows path: a
// drive letter and a colon, or a UNC prefix. It reads the shape only, so a
// Linux path with a colon in a later component is not mistaken for one.
func IsWindowsPath(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	c := p[0] | 0x20
	return c >= 'a' && c <= 'z'
}
