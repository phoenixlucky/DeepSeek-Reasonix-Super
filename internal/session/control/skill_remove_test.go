package control

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/ext/skill"
)

func removeFixture(t *testing.T) (*Controller, *skill.Store, string) {
	t.Helper()
	home := testenv.TempDir(t)
	st := skill.New(skill.Options{HomeDir: home, DisableBuiltins: true})
	c := New(Options{AllSkillStore: st, SkillStore: st})
	path, err := c.CreateSkill("helper", skill.ScopeGlobal, "---\ndescription: a helper\n---\nbe helpful")
	if err != nil {
		t.Fatal(err)
	}
	return c, st, path
}

func TestRemoveSkillDeletesTheFolderThePersonWasShown(t *testing.T) {
	c, st, path := removeFixture(t)
	if err := c.RemoveSkill("helper", skill.ScopeGlobal, path); err != nil {
		t.Fatalf("RemoveSkill: %v", err)
	}
	if _, ok := st.Read("helper"); ok {
		t.Fatal("the skill is still listed")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("its folder should be gone: %v", err)
	}
}

func TestRemoveSkillRefusesWhenTheListWasStale(t *testing.T) {
	c, st, path := removeFixture(t)
	for _, tc := range []struct {
		name  string
		scope skill.Scope
		path  string
	}{
		{"another path of the same name", skill.ScopeGlobal, filepath.Join(filepath.Dir(path), "..", "elsewhere", "SKILL.md")},
		{"another scope", skill.ScopeProject, path},
	} {
		if err := c.RemoveSkill("helper", tc.scope, tc.path); !errors.Is(err, ErrSkillChanged) {
			t.Fatalf("%s = %v, want ErrSkillChanged", tc.name, err)
		}
	}
	if _, ok := st.Read("helper"); !ok {
		t.Fatal("a refused removal deleted the skill")
	}
}

func TestRemoveSkillRefusesWhatIsNotTheUsersToDelete(t *testing.T) {
	home := testenv.TempDir(t)
	pluginRoot := filepath.Join(testenv.TempDir(t), "pkg-skills")
	body := "---\ndescription: from a package\n---\nbody"
	if err := os.MkdirAll(filepath.Join(pluginRoot, "pkgskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, "pkgskill", skill.SkillFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	st := skill.New(skill.Options{HomeDir: home, CustomPaths: []string{pluginRoot}, PluginPaths: map[string][]string{pluginRoot: {"the-package"}}})
	c := New(Options{AllSkillStore: st, SkillStore: st})
	sk, ok := st.Read("pkgskill")
	if !ok || sk.Plugin == "" {
		t.Fatalf("fixture: package skill = %+v ok=%v", sk, ok)
	}
	if err := c.RemoveSkill("pkgskill", sk.Scope, sk.Path); !errors.Is(err, ErrSkillNotRemovable) {
		t.Fatalf("a package's skill = %v, want ErrSkillNotRemovable", err)
	}
	if _, err := os.Stat(sk.Path); err != nil {
		t.Fatalf("the package's file was touched: %v", err)
	}
	var builtin skill.Skill
	for _, b := range skill.New(skill.Options{HomeDir: home}).List() {
		if b.Scope == skill.ScopeBuiltin {
			builtin = b
			break
		}
	}
	live := skill.New(skill.Options{HomeDir: home})
	cb := New(Options{AllSkillStore: live, SkillStore: live})
	if err := cb.RemoveSkill(builtin.Name, skill.ScopeBuiltin, builtin.Path); !errors.Is(err, ErrSkillNotRemovable) {
		t.Fatalf("a built-in = %v, want ErrSkillNotRemovable", err)
	}
}

func TestRemoveSkillSaysWhenThereIsNoSuchSkill(t *testing.T) {
	c, _, _ := removeFixture(t)
	var unknown *skill.NotFoundError
	if err := c.RemoveSkill("nope", skill.ScopeGlobal, "/x"); !errors.As(err, &unknown) {
		t.Fatalf("unknown skill = %v, want *skill.NotFoundError", err)
	}
}

func TestSkillRemovalSaysWhatWouldBeDeleted(t *testing.T) {
	c, st, path := removeFixture(t)
	sk, _ := st.Read("helper")
	target, folder, ok := c.SkillRemoval(sk)
	if !ok || !folder || target != filepath.Dir(path) {
		t.Fatalf("SkillRemoval = %q folder=%v ok=%v, want the skill's folder %q", target, folder, ok, filepath.Dir(path))
	}
	builtin := skill.Skill{Name: "explore", Scope: skill.ScopeBuiltin, Path: "(builtin)"}
	if _, _, ok := c.SkillRemoval(builtin); ok {
		t.Fatal("a built-in reported a place to delete")
	}
	if _, _, ok := c.SkillRemoval(skill.Skill{Name: "p", Scope: skill.ScopeGlobal, Plugin: "pkg", Path: "/x/SKILL.md"}); ok {
		t.Fatal("a package's skill reported a place to delete")
	}
	if err := c.RemoveSkill("helper", skill.ScopeGlobal, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("the shown target %q is still there: %v", target, err)
	}
}
