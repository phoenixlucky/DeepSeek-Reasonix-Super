package history

import (
	"context"
	"path/filepath"
	"strings"

	"reasonix/internal/base/retrieval"
	"reasonix/internal/contract/provider"
	"reasonix/internal/state/historycatalog"
)

// SessionHit is one conversation whose words matched, with the best passage.
type SessionHit struct {
	Path         string
	ID           string
	Title        string
	Workspace    string
	Snippet      string
	Role         provider.Role
	MessageIndex int
	Score        float64
}

const sessionSearchPool = 4

// SearchSessions finds conversations by what was said in them, across every
// workspace this process has indexed. Archived copies and sub-agent transcripts
// are left out: neither is a conversation a person opens from a list.
func SearchSessions(ctx context.Context, query string, limit int) ([]SessionHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, contextError("query is required")
	}
	terms, err := retrieval.QueryTerms(query)
	if err != nil {
		return nil, err
	}
	limit = clamp(limit, 20, maxLimit)
	catalog := processHistoryCatalog.get()
	if catalog == nil {
		return []SessionHit{}, nil
	}
	result, err := catalog.Search(ctx, historycatalog.SearchRequest{
		Query: query, Kinds: []string{string(KindUserText), string(KindAssistantText)},
		Limit: min(limit*sessionSearchPool, historycatalog.MaxLimit),
	})
	if err != nil {
		return nil, err
	}
	text := newCandidateTexts(catalog)
	seen := map[string]bool{}
	hits := make([]SessionHit, 0, limit)
	for _, candidate := range result.Items {
		if seen[candidate.SessionPath] || candidate.Source == "archive" || isSubagentPath(candidate.SessionPath) {
			continue
		}
		body, ok := text.of(candidate)
		if !ok {
			continue
		}
		seen[candidate.SessionPath] = true
		title := candidate.SessionTitle
		if title == "" {
			title = candidate.TopicTitle
		}
		hits = append(hits, SessionHit{
			Path: candidate.SessionPath, ID: sessionID(candidate.SessionPath), Title: title,
			Workspace: candidate.WorkspaceRoot, Role: provider.Role(candidate.Role), MessageIndex: candidate.MessageIndex,
			Snippet: retrieval.MakeSnippet(body, query, terms, maxSnippet), Score: candidate.Score,
		})
		if len(hits) == limit {
			break
		}
	}
	return hits, nil
}

func isSubagentPath(path string) bool {
	return filepath.Base(filepath.Dir(path)) == filepath.Base(subagentsDir("x"))
}
