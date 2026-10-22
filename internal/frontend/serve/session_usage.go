package serve

import (
	"sort"

	"reasonix/internal/contract/pricing"
)

// SourceUsage is what one usage source (the main agent, a sub-agent, compaction)
// has spent since this session was opened in this run. Requests counts provider
// responses that reported usage with a cost quote; a request that reported none
// is in no figure here. CacheUnknown is prompt the endpoint did not split into
// cached and uncached, which is billed as uncached and left out of a hit rate.
type SourceUsage struct {
	Source           string            `json:"source"`
	Requests         int               `json:"requests"`
	PromptTokens     int               `json:"promptTokens"`
	CompletionTokens int               `json:"completionTokens"`
	CacheHit         int               `json:"cacheHit"`
	CacheMiss        int               `json:"cacheMiss"`
	CacheUnknown     int               `json:"cacheUnknown,omitempty"`
	Cost             pricing.CostQuote `json:"cost"`
}

// SessionUsage is the session's usage grouped by source, read from the same
// ledger the cost total is. It begins when the ledger does: a new, resumed or
// forked session starts it again, so these are totals since this session was
// opened in this run, not over the conversation's whole life.
type SessionUsage struct {
	Requests         int           `json:"requests"`
	PromptTokens     int           `json:"promptTokens"`
	CompletionTokens int           `json:"completionTokens"`
	CacheHit         int           `json:"cacheHit"`
	CacheMiss        int           `json:"cacheMiss"`
	CacheUnknown     int           `json:"cacheUnknown,omitempty"`
	WorkMs           int64         `json:"workMs"`
	Sources          []SourceUsage `json:"sources"`
}

// SessionUsage totals the ledger by source. WorkMs is the answered work time of
// the turns that gave a final answer; failed and cancelled turns add nothing.
func (b *Broadcaster) SessionUsage() SessionUsage {
	out := SessionUsage{Sources: []SourceUsage{}}
	if b == nil {
		return out
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out.WorkMs = b.workMs
	grouped := b.ledger.EntriesBySource()
	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		src := SourceUsage{Source: name}
		quotes := make([]pricing.CostQuote, 0, len(grouped[name]))
		for _, ent := range grouped[name] {
			src.Requests += ent.RequestCount
			src.PromptTokens += ent.PromptTokens
			src.CompletionTokens += ent.CompletionTokens
			src.CacheHit += ent.CacheHitTokens
			src.CacheMiss += ent.CacheMissTokens
			src.CacheUnknown += ent.CacheUnknownTokens
			quotes = append(quotes, ent.Quote)
		}
		sort.Slice(quotes, func(i, j int) bool { return quotes[i].ModelRef < quotes[j].ModelRef })
		src.Cost = pricing.AggregateQuotes(quotes, b.displayCurrency)
		out.Requests += src.Requests
		out.PromptTokens += src.PromptTokens
		out.CompletionTokens += src.CompletionTokens
		out.CacheHit += src.CacheHit
		out.CacheMiss += src.CacheMiss
		out.CacheUnknown += src.CacheUnknown
		out.Sources = append(out.Sources, src)
	}
	return out
}
