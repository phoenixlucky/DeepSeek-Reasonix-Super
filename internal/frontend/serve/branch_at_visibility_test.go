package serve

import (
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/state/sessionstore"
)

func TestSidebarHidesPendingBranchWithoutChangingRecoveryFiltering(t *testing.T) {
	lineage := seedRecoveryLineage(t)
	h := NewHub(HubOptions{})
	holder := hubRuntime(t, h, lineage.root)
	srv := httptest.NewServer(operatorHandler(h))
	defer srv.Close()
	before := treeFor(t, srv, "")
	for _, row := range before.Sessions {
		if row.Path == lineage.covered {
			t.Fatal("covered recovery copy appeared as a sidebar conversation")
		}
	}
	if len(before.Sessions) != 2 {
		t.Fatalf("recovery fixture rows: %s", rowSummary(before.Sessions))
	}
	pending := filepath.Join(lineage.dir, "explicit-fork.jsonl")
	if err := sessionstore.MarkCleanupPending(pending, sessionstore.ForkPendingOperation); err != nil {
		t.Fatal(err)
	}
	child := sessionOf("fork question", "fork answer")
	if err := child.SaveIfAbsent(pending); err != nil {
		t.Fatal(err)
	}
	if err := sessionstore.SaveBranchMeta(pending, sessionstore.BranchMeta{ParentID: sessionstore.BranchID(lineage.lead)}); err != nil {
		t.Fatal(err)
	}
	listed, err := sessionstore.ListSessions(lineage.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listed {
		if entry.Path == pending {
			t.Fatal("pending fork leaked through ListSessions")
		}
	}
	// Even a runtime holding the path must not bypass its pending marker.
	holder.Server.Controller().SetSessionPath(pending)
	during := treeFor(t, srv, "")
	if !reflect.DeepEqual(during.Sessions, before.Sessions) || during.Total != before.Total {
		t.Fatalf("pending fork changed the sidebar: %s", rowSummary(during.Sessions))
	}
	if err := sessionstore.ClearCleanupPending(pending); err != nil {
		t.Fatal(err)
	}
	after := treeFor(t, srv, "")
	var original []treeSession
	var branch *treeSession
	for i := range after.Sessions {
		if after.Sessions[i].Path == pending {
			branch = &after.Sessions[i]
		} else {
			original = append(original, after.Sessions[i])
		}
	}
	if branch == nil || branch.RuntimeID != holder.ID || len(branch.Copies) != 0 || after.Total != before.Total+1 {
		t.Fatalf("completed fork not a normal sidebar row: %s", rowSummary(after.Sessions))
	}
	if !reflect.DeepEqual(original, before.Sessions) {
		t.Fatalf("fork changed recovery-copy filtering: %s", rowSummary(original))
	}
}
