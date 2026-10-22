package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/base/testenv"
)

func TestProjectPersonaCannotOverrideUserOnRead(t *testing.T) {
	for _, user := range []string{"", "[agent.persona]\nprefix = 'USER'\nsuffix = 'USER TAIL'\ninclude_runtime_context = true\n"} {
		for _, readOnly := range []bool{false, true} {
			t.Run(user+map[bool]string{false: "/load", true: "/read-only"}[readOnly], func(t *testing.T) {
				home, root := testenv.TempDir(t), testenv.TempDir(t)
				t.Setenv("REASONIX_HOME", home)
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(user), 0o600); err != nil {
					t.Fatal(err)
				}
				want := LoadForEdit(UserConfigPath()).Agent.Persona
				project := "[agent.persona]\nprefix = 'REPO'\nsuffix = 'REPO TAIL'\ncomplete = true\ninclude_runtime_context = false\n"
				if err := os.WriteFile(filepath.Join(root, "reasonix.toml"), []byte(project), 0o600); err != nil {
					t.Fatal(err)
				}
				load := LoadForRoot
				if readOnly {
					load = LoadForRootReadOnly
				}
				got, err := load(root)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got.Agent.Persona, want) {
					t.Fatalf("project changed user's persona: got %+v, want %+v", got.Agent.Persona, want)
				}
				for _, ignored := range got.IgnoredProjectSettings() {
					if ignored.Key == "agent.persona" && ignored.Reason == ProjectUserOnly {
						return
					}
				}
				t.Fatal("missing agent.persona user-only diagnostic")
			})
		}
	}
}
