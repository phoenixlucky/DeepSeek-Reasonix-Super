package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"reasonix/internal/base/proc"
	"reasonix/internal/base/secrets"
)

// LauncherProof is what verifying a launcher with its arguments established:
// the exact argv that was run, and where the Windows drives are on the Linux
// side when the launcher is wsl.exe.
type LauncherProof struct {
	Argv   []string
	Drives string
}

// launchersSupported is the host gate: wsl.exe-style launchers are a Windows
// fact. A test on another OS may open it to drive the same code with a script.
var launchersSupported = runtime.GOOS == "windows"

// launcherProbeTimeout bounds every launch a launcher setting makes.
var launcherProbeTimeout = bashProbeTimeout

// launcherProofs remembers, per program file and argument list, a launch that
// worked, so one process does not start the same distro for every resolution.
var launcherProofs sync.Map

type launcherIdentity struct {
	path, args string
	size       int64
	modTime    int64
}

// launcherProbeArgv is the whole probe: the launcher, its arguments, and a
// command that does nothing and touches nothing.
func launcherProbeArgv(path string, args []string) []string {
	return append(append([]string{path}, args...), "-c", "true")
}

// proveLauncher runs the probe once, bounded by the same deadline as a plain
// bash probe. A launcher that does not answer is not retried: a distro cold
// start that exceeds the bound is a setting to look at, not a flake to hide.
func proveLauncher(path string, args []string) (LauncherProof, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return LauncherProof{}, fmt.Errorf("%s: no such executable", path)
	}
	id := launcherIdentity{path: path, args: strings.Join(args, "\x00"), size: fi.Size(), modTime: fi.ModTime().UnixNano()}
	if v, ok := launcherProofs.Load(id); ok {
		return v.(LauncherProof), nil
	}
	argv := launcherProbeArgv(path, args)
	ctx, cancel := context.WithTimeout(context.Background(), launcherProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = probeWaitDelay
	cmd.Env = secrets.ProcessEnv()
	proc.HideWindow(cmd)
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return LauncherProof{}, fmt.Errorf("%s: did not answer within %s", strings.Join(argv, " "), launcherProbeTimeout)
		}
		return LauncherProof{}, fmt.Errorf("%s: did not run a command", strings.Join(argv, " "))
	}
	proof := LauncherProof{Argv: argv[:len(argv)-2], Drives: wslDrives(path, args)}
	launcherProofs.Store(id, proof)
	return proof, nil
}

// wslDrives asks the distro itself where C:\ is mounted. The automount root is
// the distro's own setting (docker-desktop uses /mnt/host), so it is read from
// the running system and never assumed. Failing to learn it loses a sentence in
// the prompt and nothing else.
func wslDrives(path string, args []string) string {
	if !isWSLExe(path) {
		return ""
	}
	argv := append(append([]string{path}, args...), "-c", `wslpath -u 'C:\'`)
	ctx, cancel := context.WithTimeout(context.Background(), launcherProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.WaitDelay = probeWaitDelay
	cmd.Env = secrets.ProcessEnv()
	proc.HideWindow(cmd)
	out, err := cmd.Output()
	root := strings.TrimSpace(string(out))
	if err != nil || !strings.HasPrefix(root, "/") || strings.ContainsAny(root, "\r\n\x00") {
		return ""
	}
	return `C:\ is ` + root
}

// isWSLExe reports the wsl.exe Windows ships, by file identity rather than by
// spelling, as isWSLLauncherPath does for the bash.exe stub.
func isWSLExe(path string) bool {
	if runtime.GOOS != "windows" || !strings.EqualFold(filepath.Base(path), "wsl.exe") {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	ref, err := os.Stat(filepath.Join(systemWindowsDir(), "System32", "wsl.exe"))
	return err == nil && os.SameFile(fi, ref)
}

// checkLauncherSettings is the part of a launcher setting that needs no launch.
func checkLauncherSettings(prefer, path string, args []string) error {
	switch strings.ToLower(strings.TrimSpace(prefer)) {
	case "bash":
	default:
		return fmt.Errorf(`launcher arguments need prefer = "bash"`)
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("launcher arguments need a path to the program they are for")
	}
	if !filepath.IsAbs(path) || !shellLaunchPlace(runtime.GOOS == "windows", path) {
		return fmt.Errorf("%s: not an absolute path on a local volume", path)
	}
	if slices.ContainsFunc(args, func(a string) bool { return a == "" || strings.ContainsAny(a, "\x00\r\n") }) {
		return errors.New("launcher arguments cannot be empty or contain line breaks")
	}
	return nil
}

// VerifyLauncher proves a launcher with its arguments the way it will run, so
// the settings screen can show the argv that worked.
func VerifyLauncher(prefer, path string, args []string) (LauncherProof, error) {
	if !launchersSupported {
		return LauncherProof{}, errors.New("launcher arguments are only supported on Windows")
	}
	if err := checkLauncherSettings(prefer, path, args); err != nil {
		return LauncherProof{}, err
	}
	return proveLauncher(path, args)
}

func (d ShellDiscovery) resolveLauncher() Shell {
	proof, err := VerifyLauncher(d.Prefer, d.Path, d.Args)
	if err == nil {
		return Shell{Kind: ShellBash, Path: d.Path, Drives: proof.Drives}.WithArgs(d.Args)
	}
	if d.Warn != nil {
		fmt.Fprintf(d.Warn, "warning: [tools.shell] launcher arguments are not usable (%v); using auto-detection instead\n", err)
	}
	plain := d
	plain.Prefer, plain.Path, plain.Args = "", "", nil
	return plain.Resolve()
}

// Description says what a launcher shell is in the words the model needs: the
// argv that starts it and, for WSL, where the Windows drives are, as the distro
// reported them. A plain shell has nothing to add and answers "".
func (s Shell) Description() string {
	args := s.Args()
	if len(args) == 0 {
		return ""
	}
	out := strings.Join(append([]string{s.Path}, args...), " ")
	if s.Drives != "" {
		out += " (Linux under WSL; " + s.Drives + ")"
	}
	return out
}
