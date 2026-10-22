package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/state/sessionstore"
)

func sidecarMessages() []provider.Message {
	return []provider.Message{
		{Role: provider.RoleSystem, Content: "sys"},
		{Role: provider.RoleUser, Content: "task"},
		{Role: provider.RoleAssistant, Content: strings.Repeat("work line\n", 800)},
		{Role: provider.RoleUser, Content: "continue"},
		{Role: provider.RoleAssistant, Content: strings.Repeat("more work\n", 800)},
		{Role: provider.RoleUser, Content: "tail"},
		{Role: provider.RoleAssistant, Content: "ok"},
	}
}

func sidecarAgent(t *testing.T, prov provider.Provider, sess *sessionstore.Session, path string, load bool) *Agent {
	t.Helper()
	a := New(prov, tool.NewRegistry(), sess, Options{
		ContextWindow: 20_000, CompactRatio: 0.5, RecentKeep: 2,
		WorkspaceID: "ws", ModelRef: "p/m",
	}, event.Discard)
	a.BindSessionPath(path, load)
	return a
}

// bindLiveAuthority puts sess on the production fail-closed path and hands it a
// live permit for path, exactly as a controller does after a successful lease.
func bindLiveAuthority(t *testing.T, sess *sessionstore.Session, path string) {
	t.Helper()
	sess.RequireWriteAuthority()
	lease, err := sessionstore.TryAcquireSessionLease(path)
	if err != nil {
		t.Fatalf("TryAcquireSessionLease: %v", err)
	}
	t.Cleanup(lease.Release)
	auth, err := lease.IssueWriteAuthority(sessionstore.NextSessionWriteGeneration())
	if err != nil {
		t.Fatalf("IssueWriteAuthority: %v", err)
	}
	sess.BindWriteAuthority(auth)
}

// The transcript save fails closed when this controller holds no live write
// authority for the path. The context sidecar write had no such check, so a
// controller that may not write the session still installed a fold there — and,
// carrying a projection loaded before the owner's fold, it summarised the same
// span a second time and replaced the digest the owner published.
func TestCompactionSidecarIsOutsideTheSessionWriteAuthority(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")

	// The stale writer binds before anything is folded, so its in-memory
	// projection is empty while the owner's later fold lands on disk.
	staleSess := &sessionstore.Session{Messages: sidecarMessages()}
	staleSess.RequireWriteAuthority() // production write path: fail closed
	staleProv := &fakeProvider{reply: "digest written without authority"}
	stale := sidecarAgent(t, staleProv, staleSess, path, true)

	ownerProv := &fakeProvider{reply: "digest written by the owner"}
	owner := sidecarAgent(t, ownerProv, &sessionstore.Session{Messages: sidecarMessages()}, path, true)
	if _, err := owner.CompactNow(context.Background(), CompactRequest{}); err != nil {
		t.Fatalf("owner CompactNow: %v", err)
	}
	owned, ok, err := sessionstore.LoadCompactionState(path)
	if err != nil || !ok {
		t.Fatalf("load owned sidecar: ok=%v err=%v", ok, err)
	}
	if owned.Projection.SummaryHash == "" || owned.Projection.CoveredCount == 0 {
		t.Fatalf("owner fold not published: %+v", owned.Projection)
	}
	ownerVersion := owner.window().currentProjectionVersion()

	// This controller may not write the session at all.
	authErr := staleSess.Save(path)
	if authErr == nil {
		t.Fatal("setup: the stale session was allowed to write the transcript")
	}
	t.Logf("transcript save for the same session: %v", authErr)

	_, compactErr := stale.CompactNow(context.Background(), CompactRequest{})
	if compactErr == nil {
		t.Fatalf("a controller with no write authority for this session installed and published a fold; the transcript save for the same session was refused with %v", authErr)
	}
	t.Logf("stale writer refused the fold: %v", compactErr)

	after, ok, err := sessionstore.LoadCompactionState(path)
	if err != nil || !ok {
		t.Fatalf("reload sidecar: ok=%v err=%v", ok, err)
	}
	if after.Projection.SummaryHash != owned.Projection.SummaryHash ||
		after.Projection.ProjectionVersion != owned.Projection.ProjectionVersion ||
		after.Generation != owned.Generation {
		t.Fatalf("a controller with no write authority for this session replaced its installed fold: summary %q -> %q, projection version %d -> %d, generation %d -> %d, two paid summaries for one span (coverage %d/%d)",
			owned.Projection.SummaryHash, after.Projection.SummaryHash, ownerVersion, after.Projection.ProjectionVersion,
			owned.Generation, after.Generation, owned.Projection.CoveredCount, after.Projection.CoveredCount)
	}
}

// The gate is an additional one: a controller that does hold the permit still
// installs and publishes a fold exactly as before, with one summary and one
// digest on disk.
func TestCompactionFoldPublishesUnderLiveWriteAuthority(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	sess := &sessionstore.Session{Messages: sidecarMessages()}
	bindLiveAuthority(t, sess, path)
	prov := &countingProvider{reply: "digest written under a live permit"}
	a := sidecarAgent(t, prov, sess, path, true)

	if _, err := a.CompactNow(context.Background(), CompactRequest{}); err != nil {
		t.Fatalf("CompactNow under a live authority: %v", err)
	}
	if len(prov.got) != 1 {
		t.Fatalf("summaries = %d, want exactly one", len(prov.got))
	}
	disk, ok, err := sessionstore.LoadCompactionState(path)
	if err != nil || !ok {
		t.Fatalf("load sidecar: ok=%v err=%v", ok, err)
	}
	if disk.Projection.SummaryHash == "" || disk.Projection.CoveredCount == 0 {
		t.Fatalf("authorized fold not published: %+v", disk.Projection)
	}
	if disk.Projection.ProjectionVersion != 1 || disk.Generation != 1 {
		t.Fatalf("published fold version = %d generation = %d, want 1/1", disk.Projection.ProjectionVersion, disk.Generation)
	}
	if disk.LastReceipt == nil || disk.LastReceipt.Status != "applied" || disk.LastReceipt.SummaryHash != disk.Projection.SummaryHash {
		t.Fatalf("published receipt does not match the installed digest: %+v", disk.LastReceipt)
	}
	if got := a.window().currentProjectionVersion(); got != disk.Projection.ProjectionVersion {
		t.Fatalf("memory projection version = %d, disk = %d", got, disk.Projection.ProjectionVersion)
	}
}

// A refused publish is not applied, so the retry is a normal retry: it folds
// the span once under the restored permit and publishes one digest. The refusal
// itself must not leave a half-installed projection that the retry folds again.
func TestRefusedSidecarPublishFoldsOnceWhenAuthorityReturns(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "session.jsonl")
	sess := &sessionstore.Session{Messages: sidecarMessages()}
	sess.RequireWriteAuthority() // production write path, no permit yet
	prov := &countingProvider{reply: "digest once the permit is back"}
	a := sidecarAgent(t, prov, sess, path, true)

	if _, err := a.CompactNow(context.Background(), CompactRequest{}); !errors.Is(err, sessionstore.ErrSessionWriteAuthorityMissing) {
		t.Fatalf("CompactNow without a permit = %v, want %v", err, sessionstore.ErrSessionWriteAuthorityMissing)
	}
	if _, ok, err := sessionstore.LoadCompactionState(path); err != nil || ok {
		t.Fatalf("a refused publish left a sidecar on disk: ok=%v err=%v", ok, err)
	}
	if got := a.window().currentProjectionVersion(); got != 0 {
		t.Fatalf("a refused publish installed a projection in memory: version %d", got)
	}

	bindLiveAuthority(t, sess, path)
	before := len(prov.got)
	if _, err := a.CompactNow(context.Background(), CompactRequest{}); err != nil {
		t.Fatalf("CompactNow with the permit restored: %v", err)
	}
	if got := len(prov.got) - before; got != 1 {
		t.Fatalf("the retry made %d summary calls, want exactly one", got)
	}
	disk, ok, err := sessionstore.LoadCompactionState(path)
	if err != nil || !ok {
		t.Fatalf("load sidecar after the retry: ok=%v err=%v", ok, err)
	}
	if disk.Projection.ProjectionVersion != 1 || disk.Generation != 1 {
		t.Fatalf("retry published version %d generation %d, want a single fold at 1/1", disk.Projection.ProjectionVersion, disk.Generation)
	}
	if disk.LastReceipt == nil || disk.LastReceipt.Status != "applied" ||
		disk.LastReceipt.SummaryHash != disk.Projection.SummaryHash || disk.Projection.SummaryHash == "" {
		t.Fatalf("retry receipt = %+v, want one applied fold for the published digest %q", disk.LastReceipt, disk.Projection.SummaryHash)
	}
}
