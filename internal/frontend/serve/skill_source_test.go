package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/skill"
	"reasonix/internal/ext/skilllock"
	"reasonix/internal/session/control"
)

func putSkillFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func listSkills(t *testing.T, skills []skill.Skill) []skillEntry {
	t.Helper()
	ctrl := control.New(control.Options{Skills: skills})
	defer ctrl.Close()
	srv := httptest.NewServer(operatorHandler(New(ctrl, NewBroadcaster(), config.ServeConfig{})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/skills")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Skills []skillEntry `json:"skills"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got.Skills
}

// An installed copy says whether the folder it came from has moved on, so the
// Skills list can show a copy that no longer matches its source.
func TestSkillsListSaysWhetherAnInstalledCopyStillMatchesItsSource(t *testing.T) {
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	src := filepath.Join(testenv.TempDir(t), "mine")
	installed := filepath.Join(home, "skills", "mine")
	for _, dir := range []string{src, installed} {
		putSkillFile(t, filepath.Join(dir, "SKILL.md"), "---\ndescription: x\n---\nv1")
	}
	digest, _ := skilllock.Digest(src)
	if err := skilllock.Record(home, skilllock.Entry{Name: "mine", Scope: "global", Source: src, Mode: "copy", Digest: digest}); err != nil {
		t.Fatal(err)
	}
	row := []skill.Skill{{Name: "mine", Scope: skill.ScopeGlobal, Path: filepath.Join(installed, "SKILL.md")}, {Name: "stranger", Scope: skill.ScopeGlobal, Path: filepath.Join(home, "skills", "stranger", "SKILL.md")}}

	got := listSkills(t, row)
	if got[0].Source == nil || got[0].Source.State != "synced" || got[0].Source.Path != src || got[0].Source.Mode != "copy" {
		t.Fatalf("a fresh copy: %+v", got[0].Source)
	}
	if got[1].Source != nil {
		t.Fatalf("a skill the installer never recorded reports a source: %+v", got[1].Source)
	}
	putSkillFile(t, filepath.Join(src, "SKILL.md"), "---\ndescription: x\n---\nv2")
	if got := listSkills(t, row); got[0].Source.State != "source_changed" {
		t.Fatalf("after the source moved: %+v", got[0].Source)
	}
	putSkillFile(t, skilllock.Path(home), "{damaged")
	if got := listSkills(t, row); got[0].Source != nil {
		t.Fatalf("a damaged record still claimed a state: %+v", got[0].Source)
	}
}
