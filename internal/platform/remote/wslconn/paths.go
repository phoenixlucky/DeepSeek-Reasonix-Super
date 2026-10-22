package wslconn

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/platform/wsl"
)

// ErrUntranslatable: the path has no spelling on the other side, such as a
// drive that is not mounted or another distribution's share.
var ErrUntranslatable = errors.New("wslconn: path cannot be translated")

// ToLinux turns an absolute Windows path, a drive path or this distribution's
// own \\wsl.localhost share, into the path the distribution sees. The
// distribution's wslpath does it, because only it knows where drives are mounted.
func (c *Conn) ToLinux(ctx context.Context, p string) (string, error) {
	if !wsl.IsWindowsPath(p) {
		return "", fmt.Errorf("%w: %q is not an absolute Windows path", ErrUntranslatable, p)
	}
	out, err := c.wslpath(ctx, "-u", p)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(out, "/") {
		return "", fmt.Errorf("%w: %q has no absolute form in %q", ErrUntranslatable, p, c.distro)
	}
	return out, nil
}

// ToWindows is the reverse: an absolute Linux path as Windows spells it, a
// drive path where it is under a mount and this distribution's share otherwise.
func (c *Conn) ToWindows(ctx context.Context, p string) (string, error) {
	if !strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%w: %q is not an absolute Linux path", ErrUntranslatable, p)
	}
	out, err := c.wslpath(ctx, "-w", p)
	if err != nil {
		return "", err
	}
	if !wsl.IsWindowsPath(out) {
		return "", fmt.Errorf("%w: %q has no Windows form", ErrUntranslatable, p)
	}
	return out, nil
}

func (c *Conn) wslpath(ctx context.Context, flag, p string) (string, error) {
	res, err := c.Run(ctx, nil, "wslpath", flag, p)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("%w: %q", ErrUntranslatable, p)
	}
	return strings.TrimRight(string(res.Stdout), "\r\n"), nil
}
