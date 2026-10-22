package control

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/ext/skill"
	"reasonix/internal/runtime/agent/testutil"
)

func TestModuleChoicesDeclareTheirSourceWithoutGuessingNames(t *testing.T) {
	c, _, _ := newOneTurnController(t, testutil.Turn{Text: "answer"})
	testComposition(t, c)
	s := c.PresetStore()
	store := skill.New(skill.Options{ProjectRoot: s.Workspace, ReasonixHomeDir: s.Home, SuppressWarnings: true})
	var builtin string
	for _, sk := range store.CompositionSkills() {
		if sk.Scope == skill.ScopeBuiltin {
			builtin = sk.SlashName()
			break
		}
	}
	if builtin == "" {
		t.Fatal("missing built-in skill fixture")
	}
	check := func(id, want string) {
		t.Helper()
		catalog, err := c.AgentPresets()
		if err != nil {
			t.Fatal(err)
		}
		for _, choice := range catalog.Modules["skills"] {
			if choice.ID != id {
				continue
			}
			raw, _ := json.Marshal(choice)
			var fields map[string]any
			_ = json.Unmarshal(raw, &fields)
			if fields["source"] != want {
				t.Fatalf("%s source = %v, want %s", id, fields["source"], want)
			}
			return
		}
		t.Fatalf("missing choice %s", id)
	}
	check(builtin, "builtin")
	dir := filepath.Join(s.Home, "skills", builtin)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+builtin+"\ndescription: Personal override\n---\nBody"), 0o600); err != nil {
		t.Fatal(err)
	}
	check(builtin, "custom")
}

func choiceByID(t *testing.T, kind string, c *Controller, id string) (string, bool) {
	t.Helper()
	catalog, err := c.AgentPresets()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range catalog.Modules[kind] {
		if m.ID == id {
			return m.Description, true
		}
	}
	return "", false
}

func TestModuleChoicesCarryWhatTheirOwnSourceSaysTheyAreFor(t *testing.T) {
	c, _, _ := newOneTurnController(t, testutil.Turn{Text: "answer"})
	testComposition(t, c)
	store := c.PresetStore()
	dir := filepath.Join(store.Home, "skills", "tidy-notes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: tidy-notes\ndescription: Tidy the notes folder\n---\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := choiceByID(t, "skills", c, "tidy-notes"); !ok || got != "Tidy the notes folder" {
		t.Fatalf("skill choice = %q (found %v), want its own one-line description", got, ok)
	}
	got, ok := choiceByID(t, "tools", c, "bash")
	if !ok || strings.TrimSpace(got) == "" {
		t.Fatalf("tool choice bash = %q (found %v), want the first line of the tool's own description", got, ok)
	}
	if strings.Contains(got, "\n") || len([]rune(got)) > choiceMaxRunes+1 {
		t.Fatalf("description %q is not one bounded line", got)
	}
}

func TestChoiceDescriptionIsOneBoundedLine(t *testing.T) {
	long := strings.Repeat("word ", 60)
	for in, want := range map[string]string{
		"":                       "",
		"  First line.\nSecond.": "First line.",
		"single":                 "single",
	} {
		if got := choiceDescription(in); got != want {
			t.Errorf("choiceDescription(%q) = %q, want %q", in, got, want)
		}
	}
	if got := choiceDescription(long); len([]rune(got)) > choiceMaxRunes+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("long description = %d runes %q", len([]rune(got)), got)
	}
}

func TestAPresetStoresOnlyTheIDsItSelects(t *testing.T) {
	c, _, _ := newOneTurnController(t, testutil.Turn{Text: "answer"})
	testComposition(t, c)
	catalog, err := c.AgentPresets()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range catalog.Presets {
		for _, m := range v.Definition.Tools.Members {
			if m.Parameters != nil {
				t.Fatalf("builtin preset %s selects tool %s with parameters %v", v.Definition.ID, m.ID, m.Parameters)
			}
		}
	}
}
