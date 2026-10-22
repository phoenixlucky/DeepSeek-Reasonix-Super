package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/skill"
	"reasonix/internal/session/control"
)

type skillRemover struct {
	control.SessionAPI
	err   error
	name  string
	scope skill.Scope
	path  string
	calls int
}

func (c *skillRemover) WorkspaceRoot() string { return "/work/app" }

func (c *skillRemover) RemoveSkill(name string, scope skill.Scope, path string) error {
	c.calls++
	c.name, c.scope, c.path = name, scope, path
	return c.err
}

func removeSkillRequest(src *skillRemover, body string) *httptest.ResponseRecorder {
	return removeSkillFrom(src, body, false)
}

func removeSkillFrom(src *skillRemover, body string, device bool) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	(&Server{ctrl: src}).registerSkillRemoveRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/skills/remove", strings.NewReader(body))
	if device {
		req = req.WithContext(withDeviceReach(req.Context(), "phone-1", 1))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestAPairedDeviceCannotRemoveASkill(t *testing.T) {
	src := &skillRemover{}
	rec := removeSkillFrom(src, `{"name":"helper","scope":"global","path":"/p/SKILL.md"}`, true)
	var reason Reason
	_ = json.Unmarshal(rec.Body.Bytes(), &reason)
	if rec.Code != http.StatusNotFound || reason.Code != codeDeviceHostOnly || src.calls != 0 {
		t.Fatalf("a device: status %d code %q calls %d, want 404 %s and nothing removed", rec.Code, reason.Code, src.calls, codeDeviceHostOnly)
	}
}

func TestRemovingASkillPassesTheShownScopeAndPathToTheController(t *testing.T) {
	src := &skillRemover{}
	rec := removeSkillRequest(src, `{"name":"helper","scope":"global","path":"/home/u/.reasonix/skills/helper/SKILL.md"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s, want 204", rec.Code, rec.Body.String())
	}
	if src.name != "helper" || src.scope != skill.ScopeGlobal || src.path != "/home/u/.reasonix/skills/helper/SKILL.md" {
		t.Fatalf("controller got %q %q %q", src.name, src.scope, src.path)
	}
}

func TestRemovingASkillNeedsTheNameAndThePathItWasShown(t *testing.T) {
	for _, body := range []string{`{}`, `{"name":"helper"}`, `{"name":"helper","scope":"global"}`, `{"path":"/x"}`, `not json`} {
		src := &skillRemover{}
		rec := removeSkillRequest(src, body)
		if rec.Code != http.StatusBadRequest || src.calls != 0 {
			t.Fatalf("%q: status %d calls %d, want 400 and no removal", body, rec.Code, src.calls)
		}
	}
}

func TestSkillRemovalRefusalsKeepTheirIdentity(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{control.ErrSkillNotRemovable, http.StatusConflict, "skill.not_removable"},
		{control.ErrSkillChanged, http.StatusConflict, "skill.changed"},
		{&skill.NotFoundError{Name: "helper"}, http.StatusNotFound, "request.not_found"},
		{errors.New("disk on fire"), http.StatusInternalServerError, "skill.remove_failed"},
	} {
		rec := removeSkillRequest(&skillRemover{err: tc.err}, `{"name":"helper","scope":"global","path":"/p/SKILL.md"}`)
		var reason Reason
		_ = json.Unmarshal(rec.Body.Bytes(), &reason)
		if rec.Code != tc.status || reason.Code != tc.code {
			t.Fatalf("%v: status %d code %q, want %d %s", tc.err, rec.Code, reason.Code, tc.status, tc.code)
		}
	}
}

func TestEachListedSkillSaysExactlyWhatRemovingItDeletes(t *testing.T) {
	home := testenv.TempDir(t)
	st := skill.New(skill.Options{HomeDir: home, DisableBuiltins: true})
	ctrl := control.New(control.Options{AllSkillStore: st, SkillStore: st})
	defer ctrl.Close()
	folderPath, err := ctrl.CreateSkill("folder-skill", skill.ScopeGlobal, "---\ndescription: in a folder\n---\nbody")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(folderPath))
	flat := filepath.Join(root, "flat-skill.md")
	if err := os.WriteFile(flat, []byte("---\ndescription: one file\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: top-skill\ndescription: at the root\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	by := map[string]skillEntry{}
	for _, e := range got.Skills {
		by[e.Name] = e
	}
	want := map[string]struct {
		removes string
		folder  bool
	}{
		"folder-skill": {filepath.Dir(folderPath), true},
		"flat-skill":   {flat, false},
		"top-skill":    {filepath.Join(root, "SKILL.md"), false},
	}
	for name, w := range want {
		e, ok := by[name]
		if !ok {
			t.Fatalf("%s is not listed: %+v", name, got.Skills)
		}
		if e.Removes != w.removes || e.RemovesFolder != w.folder {
			t.Errorf("%s: removes %q folder=%v, want %q folder=%v", name, e.Removes, e.RemovesFolder, w.removes, w.folder)
		}
	}
	if err := ctrl.RemoveSkill("top-skill", skill.ScopeGlobal, by["top-skill"].Path); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{flat, folderPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("removing the root-level skill took %s with it: %v", p, err)
		}
	}
}
