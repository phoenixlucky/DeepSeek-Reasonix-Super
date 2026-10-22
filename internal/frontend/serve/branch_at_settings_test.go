package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
	"reasonix/internal/state/store"
)

type branchSettingsResolver struct {
	*provider.StaticResolver
	beforeResolve func() error
}

func (r *branchSettingsResolver) Resolve(selection provider.Selection) (provider.Provider, error) {
	if r.beforeResolve != nil {
		if err := r.beforeResolve(); err != nil {
			return nil, err
		}
	}
	return r.StaticResolver.Resolve(selection)
}

func TestHubBranchAtFallsBackWhenSavedModelIsUnavailable(t *testing.T) {
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	root := testenv.TempDir(t)
	path := sessionstore.NewSessionPath(SessionDirFor(root), "branch")
	if err := sessionstore.NewSession("sys").SaveIfAbsent(path); err != nil {
		t.Fatal(err)
	}
	if err := sessionstore.SaveBranchMeta(path, sessionstore.BranchMeta{
		ParentID: "parent", Model: "home/removed-model", AgentPreset: "balanced", Mode: "agent",
	}); err != nil {
		t.Fatal(err)
	}
	var notices []event.Event
	h := NewHub(HubOptions{
		ProviderResolver: homeResolver(),
		DecorateSink: func(sink event.Sink) event.Sink {
			return event.FuncSink(func(e event.Event) {
				if e.Kind == event.Notice && e.Code == event.NoticeCodeBranchModelUnavailable {
					notices = append(notices, e)
				}
				sink.Emit(e)
			})
		},
	})
	defer h.Shutdown()
	rt, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: path})
	if err != nil {
		t.Fatal(err)
	}
	ctrl := rt.Server.Controller()
	if ctrl.ModelRef() != "home/chat-b" || ctrl.AgentPreset() != "balanced" || ctrl.PlanMode() {
		t.Fatalf("fallback settings = model=%q preset=%q plan=%v", ctrl.ModelRef(), ctrl.AgentPreset(), ctrl.PlanMode())
	}
	if len(notices) != 1 || notices[0].Detail != "saved branch model: home/removed-model" {
		t.Fatalf("fallback notices = %+v, want one notice naming the saved model", notices)
	}
}

func TestHubBranchAtInheritsLiveSettingsAfterDurableSave(t *testing.T) {
	writeServeModelConfig(t)
	closeSharedCatalogsOnCleanup(t)
	resolver := &branchSettingsResolver{StaticResolver: homeResolver()}
	resolver.Providers["home/chat-a"] = &turnIdentityProvider{}
	h := NewHub(HubOptions{ProviderResolver: resolver})
	defer h.Shutdown()
	root := testenv.TempDir(t)
	sourcePath := sessionstore.NewSessionPath(SessionDirFor(root), "source")
	if err := sessionstore.NewSession("sys").SaveIfAbsent(sourcePath); err != nil {
		t.Fatal(err)
	}
	source, err := h.Open(context.Background(), OpenRequest{Root: root, Model: "home/chat-a", SessionPath: sourcePath})
	if err != nil {
		t.Fatal(err)
	}
	ctrl := source.Server.Controller()
	if err := ctrl.Run(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	ctrl.SetAgentPreset("delivery")
	ctrl.SetPlanMode(true)
	parent := ctrl.SessionPath()
	oldMeta, _, err := sessionstore.LoadBranchMeta(parent)
	if err != nil {
		t.Fatal(err)
	}
	oldMeta.AgentPreset, oldMeta.Mode = "balanced", "agent"
	if err := sessionstore.SaveBranchMeta(parent, oldMeta); err != nil {
		t.Fatal(err)
	}
	cp := ctrl.Checkpoints()[0]
	var prepared atomic.Bool
	resolver.beforeResolve = func() error {
		if len(h.List()) != 1 {
			return fmt.Errorf("child runtime published before construction finished")
		}
		sessions, err := sessionstore.ListSessions(filepath.Dir(parent))
		if err != nil {
			return err
		}
		for _, session := range sessions {
			if session.Path == parent {
				continue
			}
			meta, ok, err := sessionstore.LoadBranchMeta(session.Path)
			if err != nil {
				return err
			}
			if !ok || meta.ParentID != sessionstore.BranchID(parent) {
				continue
			}
			if meta.Model != "home/chat-a" || meta.AgentPreset != "delivery" || meta.Mode != "plan" || sessionstore.IsCleanupPending(session.Path) {
				return fmt.Errorf("fork settings not durably committed before provider creation: %+v", meta)
			}
			if _, err := os.Stat(filepath.Join(store.SessionCheckpointDir(session.Path), fmt.Sprintf("turn-%d.json", cp.Turn))); err != nil {
				return err
			}
			prepared.Store(true)
			return nil
		}
		return fmt.Errorf("fork is not complete before provider creation")
	}
	body, _ := json.Marshal(map[string]any{"turn": cp.Turn, "name": "source · branch"})
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	response, err := http.Post(srv.URL+"/rt/"+source.ID+"/branch/at", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("branch status=%d", response.StatusCode)
	}
	opened, err := h.Open(context.Background(), OpenRequest{Root: root, SessionPath: result.Path})
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Load() {
		t.Fatal("branch was not prepared before opening")
	}
	child := opened.Server.Controller()
	meta, _, err := sessionstore.LoadBranchMeta(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if child == ctrl || child.ModelRef() != "home/chat-a" || child.AgentPreset() != "delivery" || !child.PlanMode() || meta.Model != child.ModelRef() || meta.AgentPreset != child.AgentPreset() || meta.Mode != "plan" {
		t.Fatalf("metadata/runtime mismatch: meta=%+v model=%s preset=%s plan=%v", meta, child.ModelRef(), child.AgentPreset(), child.PlanMode())
	}
	resolver.beforeResolve = nil
	mirror := NewHub(HubOptions{ProviderResolver: resolver})
	readOnly, err := mirror.Open(context.Background(), OpenRequest{Root: root, SessionPath: result.Path})
	if err != nil {
		mirror.Shutdown()
		t.Fatal(err)
	}
	if !readOnly.view().ReadOnly || readOnly.Server.Controller().ModelRef() != "home/chat-a" || readOnly.Server.Controller().AgentPreset() != "delivery" || !readOnly.Server.Controller().PlanMode() {
		mirror.Shutdown()
		t.Fatal("read-only branch attachment lost source settings")
	}
	mirror.Shutdown()
	child.SetPlanMode(false)
	child.SetAgentPreset("balanced")
	if !ctrl.PlanMode() || ctrl.AgentPreset() != "delivery" {
		t.Fatal("child reused source runtime state")
	}
	child.SetAgentPreset("delivery")
	child.SetPlanMode(true)
	resolver.beforeResolve = nil
	if err := h.Close(opened.ID); err != nil {
		t.Fatal(err)
	}
	holder, err := h.Open(context.Background(), OpenRequest{Root: root, Model: "home/chat-b"})
	if err != nil {
		t.Fatal(err)
	}
	if status, err := holder.Server.resumeInto(result.Path); err != nil {
		t.Fatalf("resume: status=%d err=%v", status, err)
	}
	resumed := holder.Server.Controller()
	if resumed.ModelRef() != "home/chat-a" || resumed.AgentPreset() != "delivery" || !resumed.PlanMode() {
		t.Fatalf("reused pane lost branch settings: model=%s preset=%s plan=%v", resumed.ModelRef(), resumed.AgentPreset(), resumed.PlanMode())
	}
}
