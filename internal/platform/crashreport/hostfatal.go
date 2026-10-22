package crashreport

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

const (
	hostFatalPrefix  = "host-"
	keepFatalLogs    = 5
	maxFatalLogBytes = 256 << 10
)

// InstallFatalLog mirrors what the Go runtime prints when it kills the process
// into dir/host-<utc time>-<version>-<pid>.log, for people and the shell to read.
// release removes the file of a clean run; exited keeps it for a run that returned
// a failure code, which the runtime prints nothing for, with the code and reason.
func InstallFatalLog(dir, version string) (release func(), exited func(code int, reason string)) {
	release = func() {}
	exited = func(int, string) {}
	if strings.TrimSpace(dir) == "" {
		return release, exited
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return release, exited
	}
	pruneFatalLogs(dir)
	name := fmt.Sprintf("%s%s-%s-%d%s", hostFatalPrefix, time.Now().UTC().Format("20060102T150405Z"), pathSafe(version), os.Getpid(), fatalSuffix)
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return release, exited
	}
	installErr := debug.SetCrashOutput(f, debug.CrashOptions{})
	_ = f.Close()
	if installErr != nil {
		_ = os.Remove(path)
		return release, exited
	}
	release = func() {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		_ = os.Remove(path)
	}
	exited = func(code int, reason string) {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		w, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return
		}
		defer w.Close()
		_, _ = fmt.Fprintf(w, "host exited with code %d: %s\n", code, strings.TrimSpace(reason))
	}
	return release, exited
}

// pruneFatalLogs drops empty files, keeps the newest keepFatalLogs-1 (room is left for
// the file about to be opened) and cuts each survivor to its last
// maxFatalLogBytes, which is where the cause of a crash is printed.
func pruneFatalLogs(dir string) {
	names, _ := filepath.Glob(filepath.Join(dir, hostFatalPrefix+"*"+fatalSuffix))
	type entry struct {
		path string
		at   time.Time
	}
	var kept []entry
	for _, path := range names {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Size() == 0 {
			_ = os.Remove(path)
			continue
		}
		kept = append(kept, entry{path, info.ModTime()})
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].at.After(kept[j].at) })
	for i, e := range kept {
		if i >= keepFatalLogs-1 {
			_ = os.Remove(e.path)
			continue
		}
		cutToTail(e.path)
	}
}

func cutToTail(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= maxFatalLogBytes {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	tail := make([]byte, maxFatalLogBytes)
	n, _ := f.ReadAt(tail, info.Size()-maxFatalLogBytes)
	_ = f.Close()
	if os.WriteFile(path, tail[:n], 0o600) == nil {
		_ = os.Chtimes(path, info.ModTime(), info.ModTime())
	}
}

func pathSafe(s string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.':
			return r
		}
		return '-'
	}, strings.TrimSpace(s))
	if out == "" {
		return "unknown"
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}
