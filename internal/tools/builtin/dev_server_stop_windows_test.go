//go:build windows

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/safety/sandbox"
	"reasonix/internal/tools/jobs"
)

const devServerJS = `const {spawn} = require('child_process');
const fs = require('fs');
const child = spawn(process.execPath, ['-e', "setInterval(()=>{},1000)"], {stdio: 'inherit'});
fs.writeFileSync('pids.txt', process.pid + ',' + child.pid);
require('http').createServer(()=>{}).listen(0);
setInterval(()=>{}, 1000);
`

// devServerProject lays down what "npm run dev" is: npm, then node, then a node
// child of that, all holding the same output pipe.
func devServerProject(t *testing.T) (dir string) {
	t.Helper()
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not found")
	}
	dir = testenv.TempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "server.js"), []byte(devServerJS), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"dev","scripts":{"dev":"node server.js"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func devServerPIDs(t *testing.T, dir string) []int {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(filepath.Join(dir, "pids.txt")); err == nil && strings.Contains(string(b), ",") {
			var pids []int
			for s := range strings.SplitSeq(strings.TrimSpace(string(b)), ",") {
				n, _ := strconv.Atoi(s)
				pids = append(pids, n)
			}
			return pids
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the dev server never wrote its pids")
	return nil
}

func assertGone(t *testing.T, pids []int) {
	t.Helper()
	for range 100 {
		alive := false
		for _, p := range pids {
			if windowsProcessAlive(p) {
				alive = true
			}
		}
		if !alive {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, p := range pids {
		killWindowsPID(p)
	}
	t.Fatalf("part of the dev server's process tree %v survived the stop", pids)
}

func windowsShells(t *testing.T) map[string]sandbox.Shell {
	t.Helper()
	out := map[string]sandbox.Shell{}
	if ps, err := exec.LookPath("powershell"); err == nil {
		out["powershell"] = sandbox.Shell{Kind: sandbox.ShellPowerShell, Path: ps}
	}
	if sh := sandbox.ResolveShell("bash", "", nil); sh.Kind == sandbox.ShellBash && sh.Path != "" {
		out["git-bash"] = sh
	}
	if len(out) == 0 {
		t.Skip("no shell found")
	}
	return out
}

// Stop on a turn running a dev server in the foreground returns, and takes the
// whole npm -> node -> node tree with it.
func TestStopEndsAForegroundDevServerTree(t *testing.T) {
	for name, sh := range windowsShells(t) {
		t.Run(name, func(t *testing.T) {
			dir := devServerProject(t)
			args, _ := json.Marshal(map[string]any{"command": fmt.Sprintf(`cd "%s" && npm run dev`, filepath.ToSlash(dir))})
			if name == "powershell" {
				args, _ = json.Marshal(map[string]any{"command": fmt.Sprintf(`Set-Location -LiteralPath '%s'; npm run dev`, dir)})
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := (bash{shell: sh, workDir: dir}).Execute(ctx, args); done <- err }()
			pids := devServerPIDs(t, dir)
			start := time.Now()
			cancel()
			select {
			case <-done:
			case <-time.After(40 * time.Second):
				for _, p := range pids {
					killWindowsPID(p)
				}
				t.Fatal("Stop did not finish within 40s")
			}
			t.Logf("stopped in %v", time.Since(start))
			assertGone(t, pids)
		})
	}
}

// A dev server started as a background job leaves the running list once it is
// killed, and its whole tree goes with it.
func TestKillingABackgroundDevServerEndsTheJobAndTheTree(t *testing.T) {
	for name, sh := range windowsShells(t) {
		t.Run(name, func(t *testing.T) {
			dir := devServerProject(t)
			command := fmt.Sprintf(`cd "%s" && npm run dev`, filepath.ToSlash(dir))
			if name == "powershell" {
				command = fmt.Sprintf(`Set-Location -LiteralPath '%s'; npm run dev`, dir)
			}
			m := jobs.NewManager(event.Discard)
			defer m.Close()
			ctx := jobs.WithManager(context.Background(), m)
			args, _ := json.Marshal(map[string]any{"command": command, "run_in_background": true})
			if _, err := (bash{shell: sh, workDir: dir}).Execute(ctx, args); err != nil {
				t.Fatalf("start: %v", err)
			}
			pids := devServerPIDs(t, dir)
			id := m.Running()[0].ID
			start := time.Now()
			if _, err := (killShell{}).Execute(ctx, []byte(`{"job_id":"`+id+`"}`)); err != nil {
				t.Fatalf("kill_shell: %v", err)
			}
			for len(m.Running()) > 0 {
				if time.Since(start) > 40*time.Second {
					for _, p := range pids {
						killWindowsPID(p)
					}
					t.Fatalf("the killed job is still listed after 40s: %+v", m.Running())
				}
				time.Sleep(200 * time.Millisecond)
			}
			t.Logf("left the list in %v", time.Since(start))
			assertGone(t, pids)
		})
	}
}
