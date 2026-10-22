package history

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/historycatalog"
	"reasonix/internal/state/sessionstore"
)

func saveSession(t *testing.T, path string, messages ...provider.Message) {
	t.Helper()
	s := sessionstore.NewSession("")
	for _, m := range messages {
		s.Add(m)
	}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
}

func TestSearchSessionsFindsWordsInAnyIndexedWorkspaceOncePerSession(t *testing.T) {
	prevOpen := processHistoryCatalog.open
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = processHistoryCatalog.close(ctx)
		processHistoryCatalog = indexedCatalogManager{open: prevOpen}
	})
	processHistoryCatalog.open = func(ctx context.Context, opts historycatalog.Options) (*historycatalog.Catalog, error) {
		opts.Path, opts.InMemory = "", true
		return historycatalog.Open(ctx, opts)
	}
	a, b := testenv.TempDir(t), testenv.TempDir(t)
	first := filepath.Join(a, "alpha.jsonl")
	second := filepath.Join(b, "beta.jsonl")
	saveSession(t, first,
		provider.Message{Role: provider.RoleUser, Content: "how do we rotate the zebra credentials?"},
		provider.Message{Role: provider.RoleAssistant, Content: "Rotate the zebra credentials monthly."})
	saveSession(t, second, provider.Message{Role: provider.RoleUser, Content: "zebra stripes are unrelated here"})
	processHistoryCatalog.register([]historycatalog.Root{
		{Path: a, Source: "project", Scope: "project", WorkspaceRoot: a},
		{Path: b, Source: "project", Scope: "project", WorkspaceRoot: b},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waitManagerOpen(t, &processHistoryCatalog)

	var hits []SessionHit
	for {
		var err error
		if hits, err = SearchSessions(ctx, "zebra credentials", 10); err != nil {
			t.Fatal(err)
		}
		if len(hits) == 2 {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("index never surfaced both sessions, have %d", len(hits))
		}
		time.Sleep(10 * time.Millisecond)
	}
	paths := map[string]int{}
	for _, h := range hits {
		paths[h.Path]++
		if h.Snippet == "" || h.ID == "" {
			t.Fatalf("hit without a passage or id: %#v", h)
		}
	}
	if paths[first] != 1 || paths[second] != 1 || len(hits) != 2 {
		t.Fatalf("hits per session = %v, want one each across both workspaces", paths)
	}
	if hits[0].Path != first {
		t.Fatalf("the session matching both words should rank first, got %s", hits[0].Path)
	}
	if _, err := SearchSessions(ctx, "  ", 10); err == nil {
		t.Fatal("an empty query must be refused")
	}
}
