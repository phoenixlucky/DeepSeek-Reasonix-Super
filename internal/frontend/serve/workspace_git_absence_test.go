package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/platform/gitcmd"
	"reasonix/internal/session/control"
)

func TestWorkspaceGitSaysWhyAMarkedFolderHasNoRepository(t *testing.T) {
	root := testenv.TempDir(t)
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{
		Runner: fakeRunner{}, Sink: bc, WorkspaceRoot: root,
		WorkspaceRepo: gitcmd.Repo{Dir: root, Absent: gitcmd.AbsenceRefused, AbsentDetail: "fatal: detected dubious ownership"},
	})
	defer ctrl.Close()
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/workspace/git")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got workspaceGitView
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Repo || got.Absent != "refused" || got.Detail != "fatal: detected dubious ownership" {
		t.Fatalf("view = %+v, want no repo, refused, with what git printed", got)
	}
}
