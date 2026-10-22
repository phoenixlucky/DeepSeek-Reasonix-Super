package serve

import (
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/pricing"
	"reasonix/internal/contract/provider"
)

func usageEvent(source string, prompt, completion, hit, miss int) event.Event {
	tokens := pricing.UsageTokens{PromptTokens: prompt, CompletionTokens: completion, CacheHitTokens: hit, CacheMissTokens: miss}
	quote := pricing.BuildQuote(pricing.QuoteInput{Usage: tokens, Rates: pricing.RateCard{CacheHit: .02, Input: 1, Output: 4, Currency: "CNY"}, ModelRef: "m", UsageSource: source})
	return event.Event{Kind: event.Usage, Usage: &provider.Usage{PromptTokens: prompt, CompletionTokens: completion, CacheHitTokens: hit, CacheMissTokens: miss}, CostQuote: &quote}
}

// The overview reads the ledger the cost total is read from: the same requests,
// grouped by source, with the cache the endpoint did not split kept apart.
func TestSessionUsageGroupsTheLedgerBySourceAndKeepsUnknownCacheApart(t *testing.T) {
	bc := NewBroadcaster()
	bc.Emit(usageEvent("executor", 1000, 100, 800, 200))
	bc.Emit(usageEvent("executor", 2000, 50, 1500, 500))
	bc.Emit(usageEvent("compaction", 5000, 300, 0, 0))
	bc.Emit(event.Event{Kind: event.TurnDone, WorkDurationMs: 4200})
	bc.Emit(event.Event{Kind: event.TurnDone, WorkDurationMs: 0, Cancelled: true})
	bc.Emit(event.Event{Kind: event.TurnDone, WorkDurationMs: 1800})

	got := bc.SessionUsage()
	if got.Requests != 3 || got.PromptTokens != 8000 || got.CompletionTokens != 450 || got.WorkMs != 6000 {
		t.Fatalf("totals = %+v", got)
	}
	if len(got.Sources) != 2 || got.Sources[0].Source != "compaction" || got.Sources[1].Source != "executor" {
		t.Fatalf("sources = %+v, want compaction then executor", got.Sources)
	}
	main := got.Sources[1]
	if main.Requests != 2 || main.CacheHit != 2300 || main.CacheMiss != 700 || main.CacheUnknown != 0 || main.PromptTokens != 3000 {
		t.Fatalf("executor = %+v", main)
	}
	fold := got.Sources[0]
	if fold.Requests != 1 || fold.CacheUnknown != 5000 || fold.CacheHit != 0 {
		t.Fatalf("compaction = %+v, want its undivided prompt held as unknown", fold)
	}
	if got.CacheUnknown != 5000 || got.CacheHit != 2300 {
		t.Fatalf("session cache = hit %d unknown %d", got.CacheHit, got.CacheUnknown)
	}
	if main.Cost.Coverage == "" {
		t.Fatalf("a source's cost must carry its coverage: %+v", main.Cost)
	}
	// The total the cost shows and the sources here are the same requests.
	sum := bc.SessionCostQuote()
	if sum.Coverage != main.Cost.Coverage && sum.Coverage == "" {
		t.Fatalf("session quote has no coverage: %+v", sum)
	}
}

func TestSessionUsageStartsAgainWhenTheSessionDoes(t *testing.T) {
	bc := NewBroadcaster()
	bc.Emit(usageEvent("executor", 100, 10, 50, 50))
	bc.Emit(event.Event{Kind: event.TurnDone, WorkDurationMs: 900})
	bc.ResetSession()
	got := bc.SessionUsage()
	if got.Requests != 0 || got.WorkMs != 0 || len(got.Sources) != 0 {
		t.Fatalf("after a reset: %+v", got)
	}
}
