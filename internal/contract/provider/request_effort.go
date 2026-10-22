package provider

// RequestEffortProvider reports the reasoning depths a request may carry in
// Request.EffortOverride. A provider that implements it accepts a depth per
// request, so a session can change depth without rebuilding the provider; one
// that does not (or reports none) takes only the effort it was built with.
type RequestEffortProvider interface {
	RequestEfforts() []string
}
