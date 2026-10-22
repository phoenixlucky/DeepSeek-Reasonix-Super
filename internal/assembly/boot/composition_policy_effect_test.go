package boot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/ext/hook"
	"reasonix/internal/runtime/agent/testutil"
	"reasonix/internal/session/control"
	"reasonix/internal/state/agentpreset"
)

func TestEffectCompositionCannotDeselectPolicyHooks(t *testing.T) {
	for _, scope := range []string{"user", "project"} {
		for _, event := range []hook.Event{hook.PreToolUse, hook.UserPromptSubmit} {
			t.Run(scope+"/"+string(event), func(t *testing.T) {
				var current *testutil.MockProvider
				kind := "composition-policy-" + scope + string(event)
				provider.Register(kind, func(provider.Config) (provider.Provider, error) { return current, nil })
				home, root := compositionTestWorkspace(t, kind)
				writeFile(t, root, "secret.txt", "SECRET-MUST-NOT-BE-READ")
				d := agentcomposition.Builtins()[0]
				d.ID = scope + ":guarded"
				empty := agentcomposition.Selection{Mode: agentcomposition.Select, Members: []agentcomposition.Module{}}
				d.Hooks, d.MCP, d.Extensions, d.Commands, d.Skills = empty, empty, empty, empty, empty
				if _, err := agentpreset.New(home, root).Save(d, ""); err != nil {
					t.Fatal(err)
				}
				options := Options{Home: home, WorkspaceRoot: root, AgentComposition: d.ID, Model: "test-model", resolvedShell: pinnedEffectShell()}
				current = testutil.NewMock("policy")
				legacy, err := BuildRuntime(t.Context(), options)
				if err != nil {
					t.Fatal(err)
				}
				old := legacy.Controller.CompositionBinding()
				legacy.Controller.Close()
				for _, stage := range []string{"fresh", "restored", "live-edit"} {
					t.Run(stage, func(t *testing.T) {
						marker := filepath.Join(root, stage+"-guard-ran")
						settings := hook.Settings{Hooks: map[hook.Event][]hook.HookConfig{event: {{Match: "read_file", Command: "echo guard-ran >> " + shellQuoteForTest(filepath.ToSlash(marker)) + "; exit 2"}}}}
						raw, _ := json.Marshal(settings)
						if stage == "live-edit" {
							if err := os.Remove(filepath.Join(home, hook.SettingsFilename)); err != nil {
								t.Fatal(err)
							}
						} else {
							writeFile(t, home, hook.SettingsFilename, string(raw))
						}
						current = testutil.NewMock("policy", call("read", "read_file", `{"path":"secret.txt"}`), testutil.Turn{Text: "done"})
						opts := options
						if stage != "fresh" {
							opts.AgentComposition = ""
							opts.Composition = old
						}
						ctrl, err := Build(t.Context(), opts)
						if err != nil {
							t.Fatal(err)
						}
						defer ctrl.Close()
						if stage == "live-edit" {
							if err := ctrl.SaveHooks(hook.ScopeGlobal, settings); err != nil {
								t.Fatal(err)
							}
						}
						ctrl.EnsureSessionPath()
						ctrl.SetToolApprovalMode(control.ToolApprovalYolo)
						runErr := ctrl.Run(t.Context(), "read the secret")
						if event == hook.PreToolUse && runErr != nil {
							t.Fatal(runErr)
						}
						if _, err := os.Stat(marker); err != nil {
							t.Errorf("policy hook never ran: %v", err)
						}
						requests := current.Requests()
						if event == hook.PreToolUse && len(requests) < 2 {
							t.Fatal("probe did not reach the refused tool result")
						}
						if event == hook.UserPromptSubmit && len(requests) != 0 {
							t.Error("blocked prompt reached provider")
						}
						for _, req := range requests {
							for _, m := range req.Messages {
								if m.Role == provider.RoleTool && strings.Contains(m.Content, "SECRET-MUST-NOT-BE-READ") {
									t.Error("empty hook selection removed the user's policy")
								}
							}
						}
						if stage != "fresh" && ctrl.CompositionBinding().Digest != old.Digest {
							t.Error("policy overlay rewrote the immutable binding")
						}
					})
				}
			})
		}
	}
}

func TestEffectCompositionKeepsPermissionAskAndDenyRules(t *testing.T) {
	for _, rule := range []string{"ask", "deny"} {
		t.Run(rule, func(t *testing.T) {
			p := testutil.NewMock("permission", call("read", "read_file", `{"path":"secret.txt"}`), testutil.Turn{Text: "done"})
			kind := "composition-permission-" + rule
			provider.Register(kind, func(provider.Config) (provider.Provider, error) { return p, nil })
			home, root := compositionTestWorkspace(t, kind)
			if err := os.WriteFile(config.RootsForHome(home).UserConfigPath(), []byte("[permissions]\n"+rule+" = [\"read_file(secret.txt)\"]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			writeFile(t, root, "secret.txt", "SECRET-PERMISSION-CONTENTS")
			d := agentcomposition.Builtins()[0]
			d.ID = "project:permission"
			d.Hooks = agentcomposition.Selection{Mode: agentcomposition.Select, Members: []agentcomposition.Module{}}
			if _, err := agentpreset.New(home, root).Save(d, ""); err != nil {
				t.Fatal(err)
			}
			approvals := make(chan string, 2)
			ctrl, err := Build(t.Context(), Options{Home: home, WorkspaceRoot: root, AgentComposition: d.ID, resolvedShell: pinnedEffectShell(), Sink: event.FuncSink(func(e event.Event) {
				if e.Kind == event.ApprovalRequest {
					approvals <- e.Approval.ID
				}
			})})
			if err != nil {
				t.Fatal(err)
			}
			defer ctrl.Close()
			ctrl.EnsureSessionPath()
			ctrl.EnableInteractiveApproval()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- ctrl.Run(ctx, "read secret.txt") }()
			if rule == "ask" {
				select {
				case id := <-approvals:
					ctrl.Approve(id, false, false, false)
				case <-ctx.Done():
					t.Fatal("ask rule did not request approval")
				}
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if rule == "deny" {
				select {
				case <-approvals:
					t.Error("deny rule asked instead of refusing")
				default:
				}
			}
			results := effectToolResults(p.Requests()[len(p.Requests())-1])
			if len(results) == 0 {
				t.Fatal("permission probe did not reach the provider result")
			}
			for _, result := range results {
				if strings.Contains(result, "SECRET-PERMISSION-CONTENTS") {
					t.Error("composition bypassed permission rule")
				}
			}
		})
	}
}

func TestEffectCompositionExtensionCannotOverridePermissionDeny(t *testing.T) {
	var current *testutil.MockProvider
	provider.Register("composition-permission-extension", func(provider.Config) (provider.Provider, error) { return current, nil })
	home, root := compositionTestWorkspace(t, "composition-permission-extension")
	if err := os.WriteFile(config.RootsForHome(home).UserConfigPath(), []byte("[permissions]\ndeny = [\"read_file(secret.txt)\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "secret.txt", "EXTENSION-MUST-NOT-REMOVE-DENY")
	installBootFakePlugin(t, home, "permission-policy", map[string]any{
		"intercepts": []string{"permission.decision"}, "capabilities": []string{"interceptors"},
		"env": map[string]string{"REASONIX_BOOT_FAKE_ALLOW_PERMISSION": "1"},
	})
	d := agentcomposition.Builtins()[0]
	d.ID = "project:extension-policy"
	if _, err := agentpreset.New(home, root).Save(d, ""); err != nil {
		t.Fatal(err)
	}
	var binding *agentcomposition.Binding
	var schemas []byte
	for _, stage := range []string{"unbound", "fresh", "restored"} {
		bound := stage != "unbound"
		current = testutil.NewMock("permission-extension", call("read", "read_file", `{"path":"secret.txt"}`), testutil.Turn{Text: "done"})
		opts := Options{Home: home, WorkspaceRoot: root, resolvedShell: pinnedEffectShell()}
		if bound {
			opts.AgentComposition = d.ID
		}
		if stage == "restored" {
			opts.AgentComposition, opts.Composition = "", binding
		}
		res, err := BuildRuntime(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if res.Extensions.Client("permission-policy") == nil {
			res.Controller.Close()
			t.Fatal("permission extension never started")
		}
		if res.Extensions.Client("permission-policy").TightenOnly() != bound {
			res.Controller.Close()
			t.Fatal("wrong extension permission ceiling")
		}
		if stage == "fresh" {
			binding = res.Controller.CompositionBinding()
		}
		if stage == "restored" && res.Controller.CompositionBinding().Digest != binding.Digest {
			res.Controller.Close()
			t.Fatal("restore changed binding")
		}
		res.Controller.EnsureSessionPath()
		err = res.Controller.Run(t.Context(), "read secret.txt")
		res.Controller.Close()
		if err != nil {
			t.Fatal(err)
		}
		requests := current.Requests()
		if len(requests) < 2 {
			t.Fatal("extension permission probe did not reach tool results")
		}
		got := strings.Join(effectToolResults(requests[len(requests)-1]), "\n")
		if bound {
			raw, _ := json.Marshal(requests[0].Tools)
			if stage == "fresh" {
				schemas = raw
			} else if string(schemas) != string(raw) {
				t.Error("restored policy ceiling changed tool schema bytes")
			}
		}
		if strings.Contains(got, "EXTENSION-MUST-NOT-REMOVE-DENY") == bound {
			t.Errorf("bound=%t, extension changed the expected host authority: %s", bound, got)
		}
	}
}
