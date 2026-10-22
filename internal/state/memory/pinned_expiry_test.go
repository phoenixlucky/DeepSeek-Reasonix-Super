package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
)

// expiryStore builds a store whose Set is loaded from the same directories, the
// way the session does it.
func expiryStore(t *testing.T) (Store, Options) {
	t.Helper()
	root := testenv.TempDir(t)
	user := filepath.Join(root, "user")
	proj := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return StoreFor(user, proj), Options{CWD: proj, UserDir: user}
}

// An explicit expiry is a hard boundary: the index and automatic recall already
// drop facts past it, so pinned guidance must not keep sending the body.
func TestExpiredPinnedFactLeavesTheStablePrefix(t *testing.T) {
	store, opts := expiryStore(t)
	recallTestWrite(t, store.GlobalDir, Memory{
		ID: "mem-exp-pin", Name: "old-language", Title: "Old language preference",
		Description: "Expired preference", Type: TypeUser,
		Scope: FactScopeGlobal, Activation: ActivationPinned,
		Body:      "respond in Klingon",
		ExpiresAt: time.Now().Add(-time.Hour),
	})

	set := Load(opts)
	if len(set.PinnedGuidance) != 0 {
		t.Fatalf("an expired pinned fact is still guidance: %+v", set.PinnedGuidance)
	}
	if got := set.StaticContext(); strings.Contains(got, "respond in Klingon") {
		t.Fatalf("the expired pinned body is still in the stable context:\n%s", got)
	}
	if cost := set.PrefixCost(); cost.Pinned != 0 {
		t.Fatalf("PrefixCost counts an expired pinned fact: %+v", cost)
	}
}

// A project pin shadows a global pin with the same identity. An expired project
// pin must not shadow a live global one, because nothing of the project fact is
// sent any more.
func TestExpiredProjectPinDoesNotShadowLiveGlobalPin(t *testing.T) {
	store, _ := expiryStore(t)
	recallTestWrite(t, store.GlobalDir, Memory{
		ID: "mem-global-live", Name: "language", Title: "Language preference",
		Description: "Live global preference", Type: TypeUser,
		Scope: FactScopeGlobal, Activation: ActivationPinned,
		Body: "respond in English",
	})
	recallTestWrite(t, store.Dir, Memory{
		ID: "mem-proj-expired", Name: "language", Title: "Language preference",
		Description: "Expired project preference", Type: TypeProject,
		Scope: FactScopeProject, Activation: ActivationPinned,
		Body:      "respond in Klingon",
		ExpiresAt: time.Now().Add(-time.Hour),
	})

	pinned, err := store.pinnedGuidanceForProject()
	if err != nil {
		t.Fatalf("pinnedGuidanceForProject: %v", err)
	}
	if len(pinned) != 1 {
		t.Fatalf("want exactly the live global pin, got %+v", pinned)
	}
	if pinned[0].ID != "mem-global-live" {
		t.Fatalf("an expired project pin shadowed the live global pin: %+v", pinned[0])
	}
	if strings.Contains(pinned[0].Body, "Klingon") {
		t.Fatalf("the expired body rode the prefix: %q", pinned[0].Body)
	}
}

// The budget exists to bound what the prefix carries, so a body that is not
// carried must not consume it.
func TestExpiredPinnedBodyDoesNotCountAgainstTheBudget(t *testing.T) {
	store, _ := expiryStore(t)
	store.PinnedBudgetChars = 200
	recallTestWrite(t, store.GlobalDir, Memory{
		ID: "mem-expired-long", Name: "expired-note", Title: "Expired note",
		Description: "Expired long note", Type: TypeUser,
		Scope: FactScopeGlobal, Activation: ActivationPinned,
		Body:      strings.Repeat("x", 400),
		ExpiresAt: time.Now().Add(-time.Hour),
	})

	if _, err := store.SaveWithOptions(Memory{
		Name: "current-note", Title: "Current note", Description: "Live note",
		Type: TypeUser, Scope: FactScopeGlobal, Activation: ActivationPinned,
		Body: "keep answers short",
	}, SaveOptions{}); err != nil {
		t.Fatalf("an expired pinned body still consumed the budget: %v", err)
	}
}

// Guard: a live pinned fact keeps riding the prefix.
func TestLivePinnedGuidanceIsUnchanged(t *testing.T) {
	store, opts := expiryStore(t)
	recallTestWrite(t, store.GlobalDir, Memory{
		ID: "mem-live-pin", Name: "language", Title: "Language preference",
		Description: "Live preference", Type: TypeUser,
		Scope: FactScopeGlobal, Activation: ActivationPinned,
		Body: "respond in English",
	})

	set := Load(opts)
	if len(set.PinnedGuidance) != 1 || !strings.Contains(set.StaticContext(), "respond in English") {
		t.Fatalf("a live pinned fact must keep riding the prefix: %+v", set.PinnedGuidance)
	}
}
