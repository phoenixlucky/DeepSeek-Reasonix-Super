package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// What the DeepSeek Search API answers with when it cannot serve, one identity
// per documented status, so a balance problem is never read as "no results".
const (
	CodeSearchBadRequest          = "search.bad_request"
	CodeSearchUnauthorized        = "search.unauthorized"
	CodeSearchInsufficientBalance = "search.insufficient_balance"
	CodeSearchRateLimited         = "search.rate_limited"
	CodeSearchUnavailable         = "search.upstream_unavailable"
)

const (
	deepseekQueryRunes = 128
	deepseekCount      = 10
	deepseekMaxBody    = 4 << 20
)

// DeepSeekAPI is the DeepSeek Search API (POST /v1/tools/search). It is billed
// per successful request, so it is a paid backend: a chain reaches it only
// after the free ones failed and the user allowed the charge.
type DeepSeekAPI struct {
	base   string
	key    func() string
	client *http.Client
}

func NewDeepSeekAPI(baseURL string, key func() string, client *http.Client) *DeepSeekAPI {
	if client == nil {
		client = http.DefaultClient
	}
	return &DeepSeekAPI{base: strings.TrimRight(baseURL, "/"), key: key, client: client}
}

func (*DeepSeekAPI) Name() string { return "deepseek_api" }
func (*DeepSeekAPI) Paid() bool   { return true }

func (d *DeepSeekAPI) Search(ctx context.Context, query string) ([]Hit, error) {
	key := ""
	if d.key != nil {
		key = d.key()
	}
	if key == "" {
		return nil, &BackendError{Backend: d.Name(), Kind: FailStatus, Status: http.StatusUnauthorized, Code: CodeSearchUnauthorized}
	}
	if utf8.RuneCountInString(query) > deepseekQueryRunes {
		query = string([]rune(query)[:deepseekQueryRunes])
	}
	body, _ := json.Marshal(map[string]any{"query": query, "count": deepseekCount})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+"/v1/tools/search", bytes.NewReader(body))
	if err != nil {
		return nil, &BackendError{Backend: d.Name(), Kind: FailTransport, Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, &BackendError{Backend: d.Name(), Kind: FailTransport, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &BackendError{Backend: d.Name(), Kind: FailStatus, Status: resp.StatusCode, Code: deepseekStatusCode(resp.StatusCode)}
	}
	var out struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
			Summary string `json:"summary"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, deepseekMaxBody)).Decode(&out); err != nil {
		return nil, &BackendError{Backend: d.Name(), Kind: FailDecode, Err: err}
	}
	hits := make([]Hit, 0, len(out.Results))
	for _, r := range out.Results {
		if r.URL == "" {
			continue
		}
		snippet := r.Snippet
		if snippet == "" {
			snippet = r.Summary
		}
		hits = append(hits, Hit{Title: r.Title, URL: r.URL, Snippet: truncateBytes(snippet, searxngMaxSnippet)})
	}
	if len(hits) == 0 {
		return nil, &BackendError{Backend: d.Name(), Kind: FailEmpty}
	}
	return hits, nil
}

func deepseekStatusCode(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return CodeSearchUnauthorized
	case http.StatusPaymentRequired:
		return CodeSearchInsufficientBalance
	case http.StatusTooManyRequests:
		return CodeSearchRateLimited
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusGatewayTimeout:
		return CodeSearchUnavailable
	default:
		if status >= 500 {
			return CodeSearchUnavailable
		}
		return CodeSearchBadRequest
	}
}

func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
