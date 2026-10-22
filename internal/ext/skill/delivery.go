// delivery.go — what finishing means for a worker, over and above answering.
package skill

// DeliveryContract is what a worker owes its caller when it is done. One member
// today; the shape is the point, because "what counts as finished" is a property
// of the worker rather than of the button that started it.
type DeliveryContract struct {
	// ReviewReport is the typed verdict this worker must submit before it may
	// finish, empty when it owes none.
	ReviewReport string
}

// The typed verdicts a worker can owe.
const (
	ReviewReportReview   = "review"
	ReviewReportSecurity = "security"
)

// DefaultReviewTask is the task a review skill runs with when the person names
// none: the skill's own default scope, the change set not yet on the main line.
const DefaultReviewTask = "Review the pending changes: everything this change touched that is not yet on the main line."

// RunsWithoutTask reports that invoking the skill with no task is meaningful: a
// review skill has a default scope of its own. Any other subagent skill is a
// worker that needs something to do.
func (s Skill) RunsWithoutTask() bool { return s.Delivery.ReviewReport != "" }
