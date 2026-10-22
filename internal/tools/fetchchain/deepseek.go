package fetchchain

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"reasonix/internal/tools/websearch"
)

// What the DeepSeek Fetch API answers with when it cannot serve, one identity
// per documented status.
const (
	CodeFetchBadRequest          = "fetch.bad_request"
	CodeFetchUnauthorized        = "fetch.unauthorized"
	CodeFetchInsufficientBalance = "fetch.insufficient_balance"
	CodeFetchRefusedByPolicy     = "fetch.refused_by_policy"
	CodeFetchNoContent           = "fetch.no_content"
	CodeFetchRateLimited         = "fetch.rate_limited"
	CodeFetchUnavailable         = "fetch.upstream_unavailable"
)

// deepseekFetchMaxContent bounds what one fetched page puts into the model's
// context. It is a protection, not a product limit: the local fetch reads 1 MiB
// before reducing, and this matches that order of size.
const (
	deepseekFetchMaxContent = 1 << 20
	deepseekFetchMaxBody    = 8 << 20
)

// DeepSeekFetch is the DeepSeek Fetch API (POST /v1/tools/fetch). It is billed
// per successful request, so it is a paid backend.
type DeepSeekFetch struct {
	base   string
	key    func() string
	client *http.Client
}

func NewDeepSeekFetch(baseURL string, key func() string, client *http.Client) *DeepSeekFetch {
	if client == nil {
		client = http.DefaultClient
	}
	return &DeepSeekFetch{base: strings.TrimRight(baseURL, "/"), key: key, client: client}
}

func (*DeepSeekFetch) Name() string { return "deepseek_api" }
func (*DeepSeekFetch) Paid() bool   { return true }

func (d *DeepSeekFetch) Fetch(ctx context.Context, url string) (Page, error) {
	key := ""
	if d.key != nil {
		key = d.key()
	}
	if key == "" {
		return Page{}, &websearch.BackendError{Backend: d.Name(), Kind: websearch.FailStatus, Status: http.StatusUnauthorized, Code: CodeFetchUnauthorized}
	}
	body, _ := json.Marshal(map[string]string{"url": url})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+"/v1/tools/fetch", bytes.NewReader(body))
	if err != nil {
		return Page{}, &websearch.BackendError{Backend: d.Name(), Kind: websearch.FailTransport, Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return Page{}, &websearch.BackendError{Backend: d.Name(), Kind: websearch.FailTransport, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Page{}, &websearch.BackendError{Backend: d.Name(), Kind: websearch.FailStatus, Status: resp.StatusCode, Code: fetchStatusCode(resp.StatusCode)}
	}
	var out struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, deepseekFetchMaxBody)).Decode(&out); err != nil {
		return Page{}, &websearch.BackendError{Backend: d.Name(), Kind: websearch.FailDecode, Err: err}
	}
	text := strings.TrimSpace(out.Content)
	if text == "" {
		return Page{}, &websearch.BackendError{Backend: d.Name(), Kind: websearch.FailEmpty}
	}
	if len(text) > deepseekFetchMaxContent {
		text = truncateBytes(text, deepseekFetchMaxContent) + "\n\n(truncated)"
	}
	return Page{Title: strings.TrimSpace(out.Title), Text: text, Status: http.StatusOK}, nil
}

func fetchStatusCode(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return CodeFetchUnauthorized
	case http.StatusPaymentRequired:
		return CodeFetchInsufficientBalance
	case http.StatusForbidden:
		return CodeFetchRefusedByPolicy
	case http.StatusNotFound:
		return CodeFetchNoContent
	case http.StatusTooManyRequests:
		return CodeFetchRateLimited
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusGatewayTimeout:
		return CodeFetchUnavailable
	default:
		if status >= 500 {
			return CodeFetchUnavailable
		}
		return CodeFetchBadRequest
	}
}
