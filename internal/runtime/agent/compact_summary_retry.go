package agent

import (
	"context"
	"errors"
	"fmt"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

// summaryTruncation is errSummaryOutputTruncated plus what was spent: the
// output cap of the attempt that was cut and whether a larger one was tried.
type summaryTruncation struct {
	Cap     int
	Retried bool
}

func (e *summaryTruncation) Error() string {
	if e.Retried {
		return fmt.Sprintf("%v: provider reached the output token limit (%d tokens) after one retry with a larger budget", errSummaryOutputTruncated, e.Cap)
	}
	return fmt.Sprintf("%v: provider reached the output token limit (%d tokens)", errSummaryOutputTruncated, e.Cap)
}

func (e *summaryTruncation) Unwrap() error { return errSummaryOutputTruncated }

var errSummaryPrefixNotDigest = fmt.Errorf("reused prefix: %w", errSummaryNotDigest)

// A summary gets at most one independent-shape fallback and one output-cap retry.
// Every attempt records its own spend; the returned usage is their aggregate.
func (a *contextWindow) summarizeOnce(ctx context.Context, fold []provider.Message, instructions string) (string, *provider.Usage, error) {
	allowPrefix, retried := true, false
	limit, above := summaryOutputMaxTokens, 0
	var total *provider.Usage
	for {
		summary, usage, err := a.summarizeAtMode(ctx, fold, instructions, limit, above, allowPrefix)
		total = mergeSamplingUsage(total, usage)
		if ctx.Err() != nil {
			return summary, total, err
		}
		if allowPrefix && (errors.Is(err, errSummaryPrefixNotDigest) || errors.Is(err, provider.ErrSummaryToolsUnsupported)) {
			allowPrefix = false
		} else {
			var cut *summaryTruncation
			if !errors.As(err, &cut) {
				return summary, total, err
			}
			if retried {
				cut.Retried = true
				return summary, total, err
			}
			// Do not erase the truncation if the larger cap cannot fit the context.
			next, nextUsage, nextErr := a.retryTruncatedSummary(ctx, fold, instructions, cut, allowPrefix)
			total = mergeSamplingUsage(total, nextUsage)
			if errors.Is(nextErr, errNoOutputHeadroom) {
				return summary, total, err
			}
			if ctx.Err() != nil {
				return next, total, nextErr
			}
			var nextCut *summaryTruncation
			if errors.As(nextErr, &nextCut) {
				nextCut.Retried = true
			}
			if !allowPrefix || (!errors.Is(nextErr, errSummaryPrefixNotDigest) && !errors.Is(nextErr, provider.ErrSummaryToolsUnsupported)) {
				return next, total, nextErr
			}
			allowPrefix, retried = false, true
			limit, above = summaryRetryOutputMaxTokens, cut.Cap
		}
		a.svc.sink.Emit(event.Event{Kind: event.CompactionProgress})
	}
}

func (a *contextWindow) retryTruncatedSummary(ctx context.Context, fold []provider.Message, instructions string, cut *summaryTruncation, allowPrefix bool) (string, *provider.Usage, error) {
	a.svc.sink.Emit(event.Event{Kind: event.CompactionProgress})
	return a.summarizeAtMode(ctx, fold, instructions, summaryRetryOutputMaxTokens, cut.Cap, allowPrefix)
}
