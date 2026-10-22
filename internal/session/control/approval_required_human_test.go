package control

import (
	"testing"
	"time"

	"reasonix/internal/safety/permission"
)

// A call that needs a person is answered by YOLO or a session grant only where its
// card says so. The origin question says it is not, so neither may stand in.
func TestAFreshHumanCallIsNotAnsweredByYoloOrASessionGrant(t *testing.T) {
	a := newApprovalManager(permission.New("ask", nil, nil, nil), ToolApprovalYolo, time.Minute, true)
	a.grantSession(permission.OriginConfirmTool, "text from web:example.com reached this call: bash touch x")
	if a.preApprovedForRequiredHuman(permission.OriginConfirmTool, "text from web:example.com reached this call: bash touch x") {
		t.Fatal("the origin question was answered without a person")
	}
	if a.preApprovedForDecisionOptions(permission.OriginConfirmTool, "s", nil, false, true) {
		t.Fatal("the origin question was answered at the prompt without a person")
	}
	if !a.preApprovedForRequiredHuman("computer_act", "com.example.Notes") {
		t.Fatal("YOLO no longer answers a computer call whose card says to use YOLO")
	}
	if !a.preApprovedForDecisionOptions("computer_act", "com.example.Notes", nil, false, true) {
		t.Fatal("YOLO no longer answers a computer call at the prompt")
	}
}
