package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/ext/skilllock"
	"reasonix/internal/session/control"
)

func skillUpdateServer(t *testing.T) (*httptest.Server, string, string, string) {
	t.Helper()
	home := testenv.TempDir(t)
	t.Setenv("REASONIX_HOME", home)
	src := testenv.TempDir(t)
	installed := filepath.Join(home, "skills", "mine")
	for _, dir := range []string{src, installed} {
		putSkillFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: mine\ndescription: d\n---\nv1\n")
	}
	digest, _ := skilllock.Digest(src)
	if err := skilllock.Record(home, skilllock.Entry{Name: "mine", Scope: "global", Source: src, Mode: "copy", Digest: digest, InstalledAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	t.Cleanup(ctrl.Close)
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)
	return srv, home, src, installed
}

func postSkill(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSkillUpdateRoutesPlanThenApplyThePlanThatWasShown(t *testing.T) {
	srv, _, src, installed := skillUpdateServer(t)
	putSkillFile(t, filepath.Join(src, "SKILL.md"), "---\nname: mine\ndescription: d\n---\nv2\n")
	status, plan := postSkill(t, srv.URL+"/skills/mine/update/plan", `{}`)
	if status != http.StatusOK || plan["planId"] == "" || len(plan["changes"].([]any)) != 1 {
		t.Fatalf("plan = %d %+v", status, plan)
	}
	if status, out := postSkill(t, srv.URL+"/skills/mine/update/apply", `{}`); status != http.StatusBadRequest || out["code"] != codeSkillUpdateBodyNoPlan {
		t.Fatalf("apply without a plan = %d %v", status, out)
	}
	if status, out := postSkill(t, srv.URL+"/skills/mine/update/apply", `{"planId":"not-the-plan"}`); status != http.StatusConflict || out["code"] != codeSkillUpdateStale {
		t.Fatalf("apply of another plan = %d %v", status, out)
	}
	if b, _ := os.ReadFile(filepath.Join(installed, "SKILL.md")); string(b) == "" || bytes.Contains(b, []byte("v2")) {
		t.Fatal("a refused apply changed the installed copy")
	}
	body, _ := json.Marshal(map[string]any{"planId": plan["planId"]})
	if status, out := postSkill(t, srv.URL+"/skills/mine/update/apply", string(body)); status != http.StatusOK || out["updated"] != true {
		t.Fatalf("apply = %d %v", status, out)
	}
	if b, _ := os.ReadFile(filepath.Join(installed, "SKILL.md")); !bytes.Contains(b, []byte("v2")) {
		t.Fatal("the copy was not updated")
	}
}

func TestSkillUpdateRoutesSayWhyByCode(t *testing.T) {
	srv, home, src, installed := skillUpdateServer(t)
	for path, want := range map[string]string{"/skills/nope/update/plan": codeSkillUpdateNotRecord, "/skills/..%2Fx/update/plan": codeSkillUpdateBadName} {
		if _, out := postSkill(t, srv.URL+path, `{}`); out["code"] != want {
			t.Errorf("%s = %v, want %s", path, out, want)
		}
	}
	putSkillFile(t, filepath.Join(src, "SKILL.md"), "v2")
	putSkillFile(t, filepath.Join(installed, "SKILL.md"), "local")
	_, plan := postSkill(t, srv.URL+"/skills/mine/update/plan", `{}`)
	body, _ := json.Marshal(map[string]any{"planId": plan["planId"]})
	if status, out := postSkill(t, srv.URL+"/skills/mine/update/apply", string(body)); status != http.StatusConflict || out["code"] != codeSkillUpdateConfirm {
		t.Fatalf("overwriting a local edit unconfirmed = %d %v", status, out)
	}
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}
	if _, out := postSkill(t, srv.URL+"/skills/mine/update/plan", `{}`); out["code"] != codeSkillUpdateGone {
		t.Fatalf("a vanished source = %v", out)
	}
	_ = home
}

// Updating an installed skill is the person's act: the routes sit behind the
// launch token every window mutation needs, which the agent never holds.
func TestSkillUpdateRoutesNeedTheLaunchToken(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	defer ctrl.Close()
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{AuthMode: "none"}).Handler())
	defer srv.Close()
	for _, route := range []string{"/skills/mine/update/plan", "/skills/mine/update/apply"} {
		if status, code := postLaunchJSON(t, srv.URL+route, `{"planId":"x"}`, nil); status != http.StatusForbidden || code != codeLaunchTokenRequired {
			t.Errorf("POST %s without the launch token = %d %q, want 403 %q", route, status, code, codeLaunchTokenRequired)
		}
	}
}
