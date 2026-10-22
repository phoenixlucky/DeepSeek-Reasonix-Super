package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/platform/worktree"
	"reasonix/internal/session/control"
)

type workspacePreparations struct {
	mu    sync.Mutex
	state preparationRegistry
}

type preparationRegistry struct {
	closed bool
	items  map[string]preparationEntry
	order  []string
}

type preparationEntry struct {
	selection worktreeSelection
	op        *control.WorkspacePreparation
}

func (s *Server) startWorkspacePreparation(w http.ResponseWriter, r *http.Request) {
	if !s.workspacePreparationAllowed(w, r) {
		return
	}
	var body struct {
		ID        string            `json:"id"`
		Selection worktreeSelection `json:"selection"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		badBody(w)
		return
	}
	if body.ID == "" || len(body.ID) > 80 || strings.ContainsAny(body.ID, "/\\\x00\r\n") || body.Selection.Ref == "" || body.Selection.SourceRoot == "" {
		refuse(w, http.StatusBadRequest, "workspace.worktree_request_invalid", "an operation id and source selection are required", nil)
		return
	}
	p := &s.preparations
	p.mu.Lock()
	if old, ok := p.state.items[body.ID]; ok {
		p.mu.Unlock()
		if !reflect.DeepEqual(old.selection, body.Selection) {
			refuse(w, http.StatusConflict, "workspace.worktree_request_invalid", "operation id belongs to another selection", nil)
			return
		}
		writeJSON(w, old.op.Status())
		return
	}
	if p.state.closed {
		p.mu.Unlock()
		refuse(w, http.StatusConflict, "workspace.worktree_stale", "the runtime has closed", nil)
		return
	}
	for _, entry := range p.state.items {
		switch entry.op.Status().Phase {
		case "ready", "failed", "cancelled":
		default:
			p.mu.Unlock()
			refuse(w, http.StatusConflict, "workspace.worktree_busy", "reconcile the existing preparation before starting another", nil)
			return
		}
	}
	if p.state.items == nil {
		p.state.items = make(map[string]preparationEntry)
	}
	for len(p.state.order) >= 32 {
		delete(p.state.items, p.state.order[0])
		p.state.order = p.state.order[1:]
	}
	ctx, stop := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
	op := control.NewWorkspacePreparation(ctx, body.ID, body.Selection.SourceRoot, body.Selection.Ref)
	p.state.items[body.ID] = preparationEntry{selection: body.Selection, op: op}
	p.state.order = append(p.state.order, body.ID)
	previous := s.ctl()
	p.mu.Unlock()
	go func() {
		defer stop()
		s.runWorkspacePreparation(op, previous, body.Selection)
	}()
	writeJSON(w, op.Status())
}

func (s *Server) workspacePreparationAllowed(w http.ResponseWriter, r *http.Request) bool {
	if s.grants.at(r).workspaceSwitch {
		return true
	}
	refuse(w, http.StatusForbidden, "workspace.changing_disabled", "this server may not change its workspace", nil)
	return false
}

func (s *Server) readWorkspacePreparation(w http.ResponseWriter, r *http.Request) {
	if !s.workspacePreparationAllowed(w, r) {
		return
	}
	s.preparations.mu.Lock()
	entry, ok := s.preparations.state.items[r.PathValue("id")]
	s.preparations.mu.Unlock()
	if !ok {
		refuse(w, http.StatusNotFound, "workspace.worktree_operation_unknown", "the operation cannot be reconciled; do not create a replacement automatically", nil)
		return
	}
	if r.Method == http.MethodPost {
		entry.op.Cancel()
	}
	writeJSON(w, entry.op.Status())
}

func (p *workspacePreparations) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.closed = true
	for _, entry := range p.state.items {
		entry.op.Cancel()
	}
}

func (s *Server) runWorkspacePreparation(op *control.WorkspacePreparation, previous control.SessionAPI, selection worktreeSelection) {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	ctx := op.Context()
	if ctx.Err() != nil {
		op.Finish(ctx.Err(), false)
		return
	}
	if s.ctl() != previous || previous.WorkspaceRoot() != selection.SourceRoot || !emptyConversation(previous) || controllerHasActiveRuntimeWork(previous) {
		op.Finish(errors.New("the source workspace or session has changed"), false)
		return
	}
	repo := workspaceRepo(previous)
	managedRoot := config.DeliveryWorktreeDir()
	result, err := worktree.CreateFromProgress(ctx, repo, managedRoot, selection.Ref, op.Update)
	if err == nil {
		op.Update(worktree.Progress{Phase: "copying_attachments", Branch: result.Branch, WorkspaceRoot: result.WorkspaceRoot})
		err = control.CarryDraftAttachments(selection.SourceRoot, result.WorkspaceRoot, selection.Attachments)
	}
	if err == nil {
		op.Update(worktree.Progress{Phase: "binding"})
		err = s.switchWorkspaceCommit(ctx, result.WorkspaceRoot, op.Publish)
	}
	cleanupFailed := false
	if op.Status().Phase != "ready" && result.WorktreeRoot != "" {
		op.Update(worktree.Progress{Phase: "cleaning"})
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		cleanupErr := worktree.DiscardUnpublished(cleanupCtx, repo, managedRoot, result)
		cancel()
		cleanupFailed = cleanupErr != nil
		if cleanupFailed {
			err = errors.Join(err, fmt.Errorf("worktree cleanup: %w", cleanupErr))
		}
	}
	op.Finish(err, cleanupFailed)
}
