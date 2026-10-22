package serve

import (
	"net/http"
	"strconv"
	"strings"

	"reasonix/internal/state/history"
)

// historySearchHit is one conversation whose words matched the query.
type historySearchHit struct {
	Path         string  `json:"path"`
	ID           string  `json:"id"`
	Title        string  `json:"title,omitempty"`
	Workspace    string  `json:"workspace,omitempty"`
	Snippet      string  `json:"snippet"`
	Role         string  `json:"role"`
	MessageIndex int     `json:"messageIndex"`
	Score        float64 `json:"score"`
}

// historySearch answers across every workspace this process has indexed. It is
// a hub route: the rail asks it with no session open.
func historySearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		refuse(w, http.StatusBadRequest, "history.query_required", "a search needs words to look for", nil)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	found, err := history.SearchSessions(r.Context(), query, limit)
	if err != nil {
		refuse(w, http.StatusInternalServerError, "history.search_failed", "the conversation index could not answer", nil)
		return
	}
	out := make([]historySearchHit, 0, len(found))
	for _, h := range found {
		out = append(out, historySearchHit{
			Path: h.Path, ID: h.ID, Title: h.Title, Workspace: h.Workspace, Snippet: h.Snippet,
			Role: string(h.Role), MessageIndex: h.MessageIndex, Score: h.Score,
		})
	}
	writeJSON(w, out)
}
