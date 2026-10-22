package evidence

import "testing"

func TestGhReadsLeaveNoUnprovenMutation(t *testing.T) {
	for command, want := range map[string]string{
		"gh pr checks 12":                MutationNone,
		"gh run list && gh issue view 3": MutationNone,
		"gh api repos/o/r/pulls":         MutationNone,
		"gh pr merge 12":                 MutationUnknown,
		"gh api repos/o/r -X PATCH":      MutationUnknown,
		"gh variable set X":              MutationUnknown,
	} {
		if got := bashMutationClass(command); got != want {
			t.Errorf("bashMutationClass(%q) = %q, want %q", command, got, want)
		}
	}
}
