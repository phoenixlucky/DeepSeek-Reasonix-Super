package hook

import "testing"

func TestRuntimeOptionsKeepHooksOffTheAgentsLauncherShell(t *testing.T) {
	if got := RuntimeOptionsForShell("bash", `C:\Program Files\Git\bin\bash.exe`); got.BashPath == "" {
		t.Fatal("a plain pinned bash must still reach hooks")
	}
	if got := RuntimeOptionsForShell("bash", `C:\Windows\System32\wsl.exe`, "-d", "x", "--", "/bin/bash"); got != (RuntimeOptions{}) {
		t.Fatalf("a launcher with arguments reached hooks as %+v; they keep their own shell", got)
	}
}
