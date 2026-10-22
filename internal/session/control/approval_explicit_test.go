package control

import (
	"testing"

	"reasonix/internal/safety/permission"
)

// The origin question names its own class, so a window renders it in the reader's
// language, and the sentence the model reads says YOLO does not answer it.
func TestTheOriginQuestionHasItsOwnApprovalClass(t *testing.T) {
	if got := ExplicitApprovalCode(permission.OriginConfirmTool, "text from web:x reached this call: bash ls"); got != originTextApproval {
		t.Fatalf("code = %q, want %q", got, originTextApproval)
	}
	if explicitApprovalReason(permission.OriginConfirmTool, "") == "" {
		t.Fatal("the class has no sentence for the model")
	}
	if !RequiresFreshHumanApprovalTool(permission.OriginConfirmTool) {
		t.Fatal("the origin question is not a fresh human approval")
	}
}
