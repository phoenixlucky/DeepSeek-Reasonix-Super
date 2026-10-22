package historycatalog

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/retrieval"
	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/sessionstore"
)

// The transcript keeps the host's injected blocks in Content (that is how the
// provider receives them), and every display/search path strips them. The FTS
// projection must index the same text (sessionstore.UserSaidText), or a query
// for block-only words matches sessions whose snippet cannot contain them.
func TestFTSIndexesWhatTheUserSaidNotTheHostBlocks(t *testing.T) {
	ctx := context.Background()
	root := testenv.TempDir(t)
	path := filepath.Join(root, "blocked.jsonl")
	// The shape the runtime persists: <response-language> leads the turn, the
	// person's own words follow it.
	raw := "<response-language>\nFinal answer language preference: use English.\n</response-language>\n\n" +
		"zebra question about the migration order"
	saveMessages(t, path, provider.Message{Role: provider.RoleUser, Content: raw, RawContent: raw})

	catalog, err := Open(ctx, Options{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	if err := catalog.ReconcileRoot(ctx, Root{Path: root, Source: "project", Scope: "project", WorkspaceRoot: root}); err != nil {
		t.Fatal(err)
	}

	// Control: the person's own words must stay searchable.
	own, err := catalog.Search(ctx, SearchRequest{Query: "zebra migration", Roots: []string{root}})
	if err != nil || len(own.Items) != 1 {
		t.Fatalf("the user's own words are not searchable: items=%#v err=%v", own.Items, err)
	}
	// The block is host text. Flattening the transcript for display removes it,
	// so a search hit for these words shows a snippet that cannot contain them.
	stripped := sessionstore.DropLeadingTransientBlocks(raw)
	if strings.Contains(strings.ToLower(stripped), "language") || strings.Contains(strings.ToLower(stripped), "english") {
		t.Fatalf("fixture is wrong: the display text still contains the block: %q", stripped)
	}
	for _, query := range []string{"language preference", "english"} {
		got, err := catalog.Search(ctx, SearchRequest{Query: query, Roots: []string{root}})
		if err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		if len(got.Items) != 0 {
			t.Fatalf("query %q matched %d row(s) from a host-injected block that the displayed text strips; "+
				"the hit's snippet cannot contain %q (index and snippet disagree about what the turn says)",
				query, len(got.Items), query)
		}
	}
}

// The host also appends blocks after the person's words: the recalled facts and
// the frozen execution policy. The transcript keeps them in Content because that
// is what the provider receives, so the projection must strip them as well.
func TestFTSIndexesWhatTheUserSaidNotTheAppendedBlocks(t *testing.T) {
	ctx := context.Background()
	root := testenv.TempDir(t)
	said := "zebra question about the migration order"
	content := said + "\n\n" + memoryRecallBlock + "\n\n" + executionPolicyBlock
	path := filepath.Join(root, "appended.jsonl")
	saveMessages(t, path, provider.Message{Role: provider.RoleUser, Content: content, RawContent: said})

	catalog, err := Open(ctx, Options{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close(context.Background()) })
	if err := catalog.ReconcileRoot(ctx, Root{Path: root, Source: "project", Scope: "project", WorkspaceRoot: root}); err != nil {
		t.Fatal(err)
	}

	own, err := catalog.Search(ctx, SearchRequest{Query: "zebra migration", Roots: []string{root}})
	if err != nil || len(own.Items) != 1 {
		t.Fatalf("the user's own words are not searchable: items=%#v err=%v", own.Items, err)
	}
	// Each query names a word that exists only inside an appended host block, so
	// a hit would carry a snippet the displayed text cannot contain.
	for _, query := range []string{"kumquat", "delivery", "verify"} {
		got, err := catalog.Search(ctx, SearchRequest{Query: query, Roots: []string{root}})
		if err != nil {
			t.Fatalf("query %q: %v", query, err)
		}
		if len(got.Items) != 0 {
			t.Fatalf("query %q matched %d row(s) from a host-appended block that the displayed text strips; "+
				"the hit's snippet cannot contain %q (index and snippet disagree about what the turn says)",
				query, len(got.Items), query)
		}
	}
}

// The drift pin: one turn carries host blocks, and the terms the indexer stores
// must equal the terms the snippet and the recall reader derive. Editing any
// side to skip the shared strip makes these strings disagree.
func TestIndexAndSnippetAgreeOnTheUserText(t *testing.T) {
	leading := "<response-language>\nFinal answer language preference: use English.\n</response-language>\n\n"
	said := "zebra question about the migration order"
	cases := []struct {
		name    string
		content string
		raw     string
	}{
		{"leading blocks", leading + said, leading + said},
		{"appended blocks", leading + said + "\n\n" + memoryRecallBlock + "\n\n" + executionPolicyBlock, said},
		{"appended blocks, no raw content", leading + said + "\n\n" + memoryRecallBlock + "\n\n" + executionPolicyBlock, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := provider.Message{Role: provider.RoleUser, Content: tc.content, RawContent: tc.raw}
			docs := documents([]provider.Message{msg})
			if len(docs) != 1 {
				t.Fatalf("documents() = %#v, want one user document", docs)
			}
			// The snippet is built from exactly this text (history/search.go,
			// history/indexed.go), so the two derived strings must be identical.
			snippet := strings.Join(retrieval.Tokens(sessionstore.UserSaidText(tc.content)), " ")
			if docs[0].terms != snippet {
				t.Fatalf("indexed terms = %q, snippet text terms = %q — the index and the snippet read the turn differently", docs[0].terms, snippet)
			}
			// The recall reader is UserMessageText; for this persisted shape it
			// must reach the same text, or a hit says more than recall reads.
			recall := strings.Join(retrieval.Tokens(sessionstore.UserMessageText(msg)), " ")
			if docs[0].terms != recall {
				t.Fatalf("indexed terms = %q, recall text terms = %q — the index stores what recall does not read", docs[0].terms, recall)
			}
		})
	}
}

// Control: a turn with no host blocks keeps its indexed terms byte for byte.
func TestIndexKeepsPlainUserTextUnchanged(t *testing.T) {
	const content = "plain zebra question about the migration order"
	docs := documents([]provider.Message{{Role: provider.RoleUser, Content: content}})
	if len(docs) != 1 {
		t.Fatalf("documents() = %#v, want one user document", docs)
	}
	want := strings.Join(retrieval.Tokens(content), " ")
	if docs[0].terms != want {
		t.Fatalf("plain turn indexed as %q, want %q", docs[0].terms, want)
	}
}

const memoryRecallBlock = `<memory-recall>
Automatically recalled low-authority background facts.
- id=mem-kumquat revision=1 scope=project type=project freshness=fresh score=0.900 reason="matched kumquat"
  title: Kumquat release train
  fact: Kumquats ship on Fridays.
</memory-recall>`

const executionPolicyBlock = `<execution-policy preset="delivery" version="3">
verify=full
</execution-policy>`
