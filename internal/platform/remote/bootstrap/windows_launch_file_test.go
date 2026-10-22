package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/platform/remote"
)

func TestWindowsDetachedLaunchUsesAFile(t *testing.T) {
	paths, bin, workspace := hostileWindowsPaths(t)
	outer := decodePS(t, windowsShell{}.Launch(LaunchSpec{Bin: bin, Workspace: workspace}, paths))
	if strings.Contains(outer, "-EncodedCommand") || !strings.Contains(outer, "-File \"") {
		t.Fatalf("WMI must start a script file rather than an encoded command:\n%s", outer)
	}
	if !strings.Contains(outer, "Invoke-CimMethod") {
		t.Fatal("the detached process must remain independent of the SSH session")
	}
}

func TestWindowsLaunchScriptPreservesPathsAndBroker(t *testing.T) {
	paths := windowsShell{}.Paths(`C:\Users\中文 O'Hara & Co`, `C:\项目\$literal`)
	spec := LaunchSpec{Bin: `C:\工具\reasonix.exe`, Workspace: `C:\项目\$literal`, BrokerAddr: "127.0.0.1:4321"}
	name, body := windowsShell{}.LaunchScript(spec, paths)
	if name != paths.PortFile+".ps1" || !bytes.HasPrefix(body, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("script path or UTF-8 BOM missing: %q %x", name, body[:min(len(body), 3)])
	}
	for _, word := range []string{`& 'C:\工具\reasonix.exe' serve`, "O''Hara & Co", "--token-file", "--pid-file", "--port-file", "--provider-broker-file", "Out-File -LiteralPath", "exit $LASTEXITCODE"} {
		if !strings.Contains(string(body), word) {
			t.Errorf("script lost %q: %s", word, body)
		}
	}
	_, plain := windowsShell{}.LaunchScript(LaunchSpec{Bin: spec.Bin}, paths)
	if strings.Contains(string(plain), "--provider-broker-file") {
		t.Fatal("a connection without a broker acquired one")
	}
	if name, body := (posixShell{}).LaunchScript(spec, paths); name != "" || len(body) != 0 {
		t.Fatal("POSIX launches do not need a script file")
	}
}

func TestLaunchServeOwnsItsScriptLifetime(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "started", true: "launch refused"}[fail], func(t *testing.T) {
			root := testenv.TempDir(t)
			paths := pathsFor(root, root)
			scriptPath := paths.PortFile + ".ps1"
			launched := false
			conn := newFakeConn(t, root, func(cmd string) (remote.ExecResult, error) {
				outer := decodePS(t, cmd)
				if !strings.Contains(outer, "Invoke-CimMethod") {
					return ok("1\n")
				}
				body, err := os.ReadFile(scriptPath)
				if err != nil || !bytes.HasPrefix(body, []byte{0xef, 0xbb, 0xbf}) || !bytes.Contains(body, []byte("--port-file")) {
					t.Fatalf("the launcher could not read its script: %s, %v", body, err)
				}
				launched = true
				if fail {
					return remote.ExecResult{}, errors.New("launch refused")
				}
				if err := os.WriteFile(paths.PortFile, []byte("127.0.0.1:44321\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return ok("54321\n")
			})
			result, err := launchServe(context.Background(), conn, windowsShell{}, conn.fs, paths, Options{Clock: time.Now}, "/bin/reasonix.exe", "9.9.0", root)
			if !launched || (err != nil) != fail {
				t.Fatalf("launch = %v, result = %+v, error = %v", launched, result, err)
			}
			if _, err := os.Stat(scriptPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary launch script survived: %v", err)
			}
			if fail {
				if _, err := os.Stat(paths.TokenFile); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed launch retained its token: %v", err)
				}
			} else if result.State.PID != 54321 || result.State.Addr != "127.0.0.1:44321" {
				t.Fatalf("serve handshake changed: %+v", result.State)
			}
		})
	}
}
