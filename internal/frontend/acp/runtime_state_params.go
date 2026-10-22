package acp

// SessionRuntimeStateParams identifies the exact controller configuration whose
// effective policy is being reported. Model/profile switches rebuild the
// controller, so status must be recomputed from the same resolved inputs.
type SessionRuntimeStateParams struct {
	Cwd            string
	Model          string
	RuntimeProfile string
	// AdditionalDirs are the vetted extra roots the session was opened with.
	AdditionalDirs []string
}
