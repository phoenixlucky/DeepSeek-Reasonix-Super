package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"reasonix/internal/session/control"
)

func startPreparation(t *testing.T, url, root, id string) control.WorkspacePreparationStatus {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "selection": worktreeSelection{Ref: "refs/heads/studio", SourceRoot: root}})
	resp, err := http.Post(url+"/workspace/prepare", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("start: %d %s", resp.StatusCode, data)
	}
	var status control.WorkspacePreparationStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func awaitPreparation(t *testing.T, url, id, phase string) control.WorkspacePreparationStatus {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		var status control.WorkspacePreparationStatus
		fetchInto(t, url+"/workspace/prepare/"+id, &status)
		if status.Phase == phase {
			return status
		}
		select {
		case <-deadline:
			t.Fatalf("want %s, got %+v", phase, status)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestWorkspacePreparationCancelRollsBackBeforeAcknowledgement(t *testing.T) {
	s, url, root := worktreeServer(t)
	old := s.Controller()
	entered := make(chan struct{})
	s.buildWorkspaceController = func(ctx context.Context, _, _ string) (*control.Controller, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	startPreparation(t, url, root, "cancel-test")
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("build did not start")
	}
	resp, err := http.Post(url+"/workspace/prepare/cancel-test/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel: %d", resp.StatusCode)
	}
	status := awaitPreparation(t, url, "cancel-test", "cancelled")
	if s.Controller() != old {
		t.Fatal("cancel replaced the source controller")
	}
	if status.WorkspaceRoot == "" || status.Branch == "" {
		t.Fatalf("missing worktree identity: %+v", status)
	}
	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil || strings.Count(string(out), "worktree ") != 1 {
		t.Fatalf("worktree leaked: %s %v", out, err)
	}
}

func TestWorkspacePreparationRetryIDReconcilesAlreadyPublishedResult(t *testing.T) {
	s, url, root := worktreeServer(t)
	startPreparation(t, url, root, "ready-test")
	status := awaitPreparation(t, url, "ready-test", "ready")
	current := s.Controller()
	repeated := startPreparation(t, url, root, "ready-test")
	if repeated.WorkspaceRoot != status.WorkspaceRoot || s.Controller() != current {
		t.Fatal("retry created another workspace")
	}
	resp, err := http.Post(url+"/workspace/prepare/ready-test/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var cancelled control.WorkspacePreparationStatus
	if err := json.NewDecoder(resp.Body).Decode(&cancelled); err != nil {
		t.Fatal(err)
	}
	if cancelled.Phase != "ready" || s.Controller() != current {
		t.Fatal("late cancel rolled back a published runtime")
	}
}

func TestWorkspacePreparationFailureCanRetryAfterCleanup(t *testing.T) {
	s, url, root := worktreeServer(t)
	old, build := s.Controller(), s.buildWorkspaceController
	s.buildWorkspaceController = func(context.Context, string, string) (*control.Controller, error) {
		return nil, errors.New("fixture build failure")
	}
	startPreparation(t, url, root, "failed-test")
	awaitPreparation(t, url, "failed-test", "failed")
	if s.Controller() != old {
		t.Fatal("failed build replaced the source controller")
	}
	s.bindMu.Lock()
	s.buildWorkspaceController = build
	s.bindMu.Unlock()
	startPreparation(t, url, root, "retry-test")
	awaitPreparation(t, url, "retry-test", "ready")
	out, err := exec.Command("git", "-C", root, "for-each-ref", "--format=%(refname)", "refs/heads/reasonix/").Output()
	if err != nil || len(strings.Fields(string(out))) != 1 {
		t.Fatalf("failed attempt left another branch: %s %v", out, err)
	}
}

func TestWorkspacePreparationCleanupFailureBlocksReplacement(t *testing.T) {
	s, url, root := worktreeServer(t)
	s.buildWorkspaceController = func(_ context.Context, target, _ string) (*control.Controller, error) {
		cmd := exec.Command("git", "-C", target, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "external change")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, errors.New(string(out))
		}
		return nil, errors.New("fixture build failure after external change")
	}
	startPreparation(t, url, root, "retained-test")
	retained := awaitPreparation(t, url, "retained-test", "cleanup_failed")
	if s.Controller().WorkspaceRoot() != root {
		t.Fatal("failed build replaced the source controller")
	}
	body, _ := json.Marshal(map[string]any{"id": "replacement", "selection": worktreeSelection{Ref: "refs/heads/studio", SourceRoot: root}})
	resp, err := http.Post(url+"/workspace/prepare", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var refusal struct{ Code string }
	if err := json.NewDecoder(resp.Body).Decode(&refusal); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusConflict || refusal.Code != "workspace.worktree_busy" {
		t.Fatalf("replacement = %d %s", resp.StatusCode, refusal.Code)
	}
	out, err := exec.Command("git", "-C", retained.WorkspaceRoot, "log", "-1", "--format=%s").Output()
	if err != nil || strings.TrimSpace(string(out)) != "external change" {
		t.Fatalf("external work was not retained: %s %v", out, err)
	}
}
