package tool

import "context"

// PaidSearchRequest is a web_search call about to use a backend the vendor
// charges for. Query is what would be sent to the vendor, shown to the user so
// they see what leaves the machine. Price says what each successful request
// costs and where that was read.
type PaidSearchRequest struct {
	// Service is what is being bought: PaidServiceSearch (the default) or
	// PaidServiceFetch. Agreeing to one is not agreeing to the other.
	Service string
	Backend string
	// Query is the search term, or for a fetch the address that would be sent.
	Query string
	Price string
}

// The services a paid backend can be asked for.
const (
	PaidServiceSearch = "search"
	PaidServiceFetch  = "fetch"
)

// PaidSearchApprover asks the user whether one backend may be charged. It is a
// fresh human decision: no approval mode answers it, and a nil approver
// (headless runs, sub-agents with no interactive parent) fails closed.
type PaidSearchApprover interface {
	ApprovePaidSearch(ctx context.Context, req PaidSearchRequest) (allow bool, err error)
}

type paidSearchApproverContextKey struct{}

// WithPaidSearchApprover stamps an interactive approver onto a tool execution
// context.
func WithPaidSearchApprover(ctx context.Context, approver PaidSearchApprover) context.Context {
	if approver == nil {
		return ctx
	}
	return context.WithValue(ctx, paidSearchApproverContextKey{}, approver)
}

// PaidSearchApproverFrom returns the approver carried by ctx.
func PaidSearchApproverFrom(ctx context.Context) (PaidSearchApprover, bool) {
	if ctx == nil {
		return nil, false
	}
	approver, ok := ctx.Value(paidSearchApproverContextKey{}).(PaidSearchApprover)
	return approver, ok && approver != nil
}
