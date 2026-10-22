package acp

import (
	"testing"

	"reasonix/internal/safety/permission"
)

// An approval name ACP has no special wording for is still a fresh approval: it is
// titled with the tool and the subject (which leads with the host's own label),
// carries no session or persistent grant, and offers allow-once or reject only.
func TestTheOriginQuestionReachesAnACPClientAsAFreshAllowOnceOrReject(t *testing.T) {
	opts := approvalOptions(permission.OriginConfirmTool, "text from web:x reached this call: bash ls", true, false)
	if len(opts) != 2 || opts[0].Kind != OptAllowOnce || opts[1].Kind != OptRejectOnce {
		t.Fatalf("options = %+v, want allow once and reject only", opts)
	}
	if got := toolKindFor(permission.OriginConfirmTool); got != "other" {
		t.Fatalf("kind = %q, want other", got)
	}
}
