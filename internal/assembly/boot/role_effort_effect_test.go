package boot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/runtime/agent/testutil"
)

// Inspect the real provider constructor payloads from Build, including the
// recovery reviewer and goal evaluator that share the guardian model.
func TestEffectRoleEffortPreservesSavedProviderSetting(t *testing.T) {
	for _, tc := range []struct {
		name, setting, want string
	}{
		{"unset keeps saved effort", "", "high"},
		{"explicit auto uses model default", "auto", "low"},
		{"explicit level overrides saved effort", "medium", "medium"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateConfigHome(t)
			root := robustTempDir(t)
			t.Chdir(root)
			kind := "boot-role-effort-" + strings.ReplaceAll(t.Name(), "/", "-")
			var mu sync.Mutex
			efforts := map[string][]string{}
			provider.Register(kind, func(pc provider.Config) (provider.Provider, error) {
				mu.Lock()
				effort, _ := pc.Extra["effort"].(string)
				efforts[pc.Model] = append(efforts[pc.Model], effort)
				mu.Unlock()
				return testutil.NewMock(pc.Model), nil
			})
			roleSettings := ""
			if tc.setting != "" {
				roleSettings = fmt.Sprintf("role_efforts = { planner = %q, guardian = %q }", tc.setting, tc.setting)
			}
			writeUserConfig(t, fmt.Sprintf(`
default_model = "roles/chat"

[codegraph]
enabled = false

[agent]
system_prompt = "BASE"
planner_model = "roles/planner"
guardian_model = "roles/guardian"
%s

[[providers]]
name = "roles"
kind = %q
models = ["chat", "planner", "guardian"]
default = "chat"
effort = "high"
default_effort = "low"
supported_efforts = ["low", "medium", "high"]
reasoning_protocol = "openai"
`, roleSettings, kind))
			approveWorkspace(t, root)
			ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
			if err != nil {
				t.Fatal(err)
			}
			defer ctrl.Close()
			mu.Lock()
			defer mu.Unlock()
			for model, count := range map[string]int{"planner": 1, "guardian": 3} {
				got := efforts[model]
				if len(got) != count {
					t.Fatalf("%s provider constructions = %d, want %d; efforts=%v", model, len(got), count, efforts)
				}
				for _, effort := range got {
					if effort != tc.want {
						t.Fatalf("%s provider effort = %q, want %q", model, effort, tc.want)
					}
				}
			}
		})
	}
}
