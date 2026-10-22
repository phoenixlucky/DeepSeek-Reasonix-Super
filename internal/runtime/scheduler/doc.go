// Package scheduler decides which scheduled runs start now and hands each to a
// supervisor. It is the one place a tick is judged, so the in-kernel ticker and
// `reasonix schedule tick` cannot differ: the store admits or refuses every claim
// (budgets, expiry, the global pause, one run at a time), and this package only
// asks it, in order.
//
// Only the latest slot of a schedule is considered. A slot that is still within
// its grace is claimed; one that is past it is recorded as skipped, so a laptop
// opened after a week yields at most one late run per schedule and only while it
// is still timely. A run that was claimed is run to its end before Tick returns.
package scheduler
