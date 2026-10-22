package boot

import (
	"errors"
	"testing"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/hook"
)

func TestCompositionHookLaunchCredentialsRespectArgumentBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  hook.HookConfig
		refused bool
	}{
		{name: "quoted drive path", config: hook.HookConfig{Command: "cat 'C:/fixture/scripts with spaces/startup.txt'"}},
		{name: "quoted native path", config: hook.HookConfig{Command: `cat 'C:\fixture\scripts with spaces\startup.txt'`}},
		{name: "exec drive path", config: hook.HookConfig{ExecutionMode: hook.ExecutionExec, Command: "cat", Argv: []string{"C:/fixture/scripts with spaces/startup.txt"}}},
		{name: "legacy argv path", config: hook.HookConfig{Command: `C:\fixture\scripts with spaces\runner.exe`, Argv: []string{"C:/fixture/startup.txt"}}},
		{name: "shell token", config: hook.HookConfig{Command: "curl --token private-fixture-secret https://example.test"}, refused: true},
		{name: "exec token", config: hook.HookConfig{ExecutionMode: hook.ExecutionExec, Command: "curl", Argv: []string{"--token", "private-fixture-secret"}}, refused: true},
		{name: "legacy argv token", config: hook.HookConfig{Command: "curl", Argv: []string{"--token", "private-fixture-secret"}}, refused: true},
		{name: "unused shell argv token", config: hook.HookConfig{ExecutionMode: hook.ExecutionShell, Command: "echo ready", Argv: []string{"--token", "private-fixture-secret"}}, refused: true},
		{name: "shell header", config: hook.HookConfig{Command: `curl -H 'Authorization: Bearer private-fixture-secret' https://example.test`}, refused: true},
		{name: "shell control", config: hook.HookConfig{Command: "echo ready; curl --token private-fixture-secret"}, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, root := compositionTestWorkspace(t, "openai")
			b := &builder{roots: config.RootsForHome(home), root: root, opts: Options{Composition: &agentcomposition.Binding{}}}
			err := b.retainHook("fixture", hook.ResolvedHook{HookConfig: tc.config})
			if tc.refused {
				if !errors.Is(err, agentcomposition.ErrUnavailable) {
					t.Fatalf("credential-bearing launch accepted: %v", err)
				}
				if len(b.opts.Composition.Resources) != 0 {
					t.Fatal("refused launch retained resources")
				}
			} else if err != nil {
				t.Fatalf("ordinary file argument rejected: %v", err)
			}
		})
	}
}
