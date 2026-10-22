package boot

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/safety/sandbox"
	"reasonix/internal/state/peermail"
)

// What a confined command may do to the peer-mail spool, composed exactly the
// way a session composes its sandbox: the workspace is the only write root and
// the forbidden roots come from the config's own composition. Sender identity
// and message confidentiality both rest on this, so it is asserted end to end.
func TestAConfinedCommandCannotTouchThePeerSpool(t *testing.T) {
	if runtime.GOOS == "windows" || !sandbox.Available() {
		t.Skip("no OS-level bash confinement on this machine")
	}
	t.Run("workspace beside the state root", func(t *testing.T) { confinedSpoolCase(t, false) })
	// The workspace being the state root's parent (a project opened at the home
	// directory) puts the spool inside a writable root; the forbidden-root rule
	// has to win there too.
	t.Run("workspace above the state root", func(t *testing.T) { confinedSpoolCase(t, true) })
}

func confinedSpoolCase(t *testing.T, workspaceAbove bool) {
	base, err := os.MkdirTemp(os.Getenv("HOME"), ".reasonix-peers-test-")
	if err != nil {
		t.Skipf("cannot create a work dir under home: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	home, workspace := filepath.Join(base, "state"), filepath.Join(base, "ws")
	for _, dir := range []string{home, workspace} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if workspaceAbove {
		workspace = base
	}
	t.Setenv("REASONIX_HOME", home)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(cfg.Roots().PeersDir(), "spool", "sess-b")
	if err := os.MkdirAll(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(spool, "msg-1.json")
	if err := os.WriteFile(entry, []byte(`{"body":"private words"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	spec := sandbox.Spec{Mode: "enforce", WriteRoots: []string{workspace}, ForbidReadRoots: cfg.ForbiddenRootsForRoot(workspace), Network: true}
	run := func(command string) (string, error) {
		argv, wrapped := sandbox.Command(spec, sandbox.Shell{Kind: sandbox.ShellBash, Path: "bash"}, command)
		if !wrapped {
			t.Fatal("the command ran unwrapped")
		}
		out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		return string(out), err
	}

	// Asserted on what the host observes, not on exit codes: bubblewrap masks a
	// forbidden directory with an empty one, so rm and mkdir succeed on the mask.
	if out, _ := run("cat " + entry); strings.Contains(out, "private words") {
		t.Errorf("a confined command read an entry: %q", out)
	}
	if out, _ := run("ls " + spool); strings.Contains(out, "msg-1.json") {
		t.Errorf("a confined command listed the spool: %q", out)
	}
	d, err := peermail.Open(cfg.Roots().PeersDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Audit(peermail.AuditRecord{Receiver: "sess-b", Sender: "sess-a", ID: "m1", Decision: peermail.DecisionHeld, SHA256: "audit-digest-marker"}); err != nil {
		t.Fatal(err)
	}
	if out, _ := run("cat " + filepath.Join(cfg.Roots().PeersDir(), "audit", "*.jsonl")); strings.Contains(out, "audit-digest-marker") {
		t.Errorf("a confined command read the audit trail: %q", out)
	}
	if out, _ := run("ls " + cfg.Roots().PeersDir()); strings.Contains(out, "spool") {
		t.Errorf("a confined command listed the peers directory: %q", out)
	}
	for _, command := range []string{
		"echo forged > " + filepath.Join(spool, "forged.json"),
		"echo forged > " + entry,
		"mv " + entry + " " + entry + ".x",
		"rm -f " + entry,
		"mkdir " + filepath.Join(cfg.Roots().PeersDir(), "spool", "sess-c"),
		"mkdir -p " + filepath.Join(cfg.Roots().PeersDir(), "live") + " && echo '{}' > " + filepath.Join(cfg.Roots().PeersDir(), "live", "x.json"),
	} {
		_, _ = run(command)
	}
	if b, err := os.ReadFile(entry); err != nil || string(b) != `{"body":"private words"}` {
		t.Fatalf("the entry changed or vanished: %q %v", b, err)
	}
	for _, path := range []string{
		filepath.Join(spool, "forged.json"),
		entry + ".x",
		filepath.Join(cfg.Roots().PeersDir(), "spool", "sess-c"),
		filepath.Join(cfg.Roots().PeersDir(), "live", "x.json"),
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("a confined command left %s on the host", path)
		}
	}
	if _, err := run("echo ok > " + filepath.Join(workspace, "ok.txt")); err != nil {
		t.Fatalf("the workspace must stay writable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "ok.txt")); err != nil {
		t.Fatalf("the workspace write did not land: %v", err)
	}
}
