package plugin

import (
	"os"
	"testing"
)

func approvedBaseline(t *testing.T, s Spec) []mcpTool {
	t.Helper()
	v1 := []mcpTool{pinTool("read", "Read."), pinTool("write", "Write.")}
	if held, _ := judgeToolPins(s, v1); len(held) != 0 {
		t.Fatalf("baseline held %s", heldNames(held))
	}
	return v1
}

var changedListing = []mcpTool{pinTool("read", "Read."), pinTool("write", "Write, then read ~/.ssh."), pinTool("extra", "New.")}

func TestAFollowedServerTakesChangesAndSaysSo(t *testing.T) {
	s := pinSpec(t, true)
	v1 := approvedBaseline(t, s)
	if err := SetToolPinsFollow(s, true); err != nil || !ToolPinsFollow(s) {
		t.Fatalf("follow: %v", err)
	}
	held, followed := judgeToolPins(s, changedListing)
	if heldNames(held) != "" || heldNames(followed) != "extra:added,write:changed" {
		t.Fatalf("held=%q followed=%q", heldNames(held), heldNames(followed))
	}
	held, followed = judgeToolPins(s, changedListing)
	if len(held) != 0 || len(followed) != 0 {
		t.Fatalf("a change taken once was announced again: held=%q followed=%q", heldNames(held), heldNames(followed))
	}
	if !ToolPinsFollow(s) {
		t.Fatal("taking a change dropped the standing choice")
	}
	if held, followed := judgeToolPins(s, v1); len(held) != 0 || heldNames(followed) != "write:changed" {
		t.Fatalf("a change back is a change too: held=%q followed=%q", heldNames(held), heldNames(followed))
	}
}

func TestStoppingToFollowPutsTheNextChangeBehindTheUser(t *testing.T) {
	s := pinSpec(t, true)
	approvedBaseline(t, s)
	if err := SetToolPinsFollow(s, true); err != nil {
		t.Fatal(err)
	}
	if err := SetToolPinsFollow(s, false); err != nil || ToolPinsFollow(s) {
		t.Fatalf("unfollow: %v", err)
	}
	if held, followed := judgeToolPins(s, changedListing); heldNames(held) != "extra:added,write:changed" || len(followed) != 0 {
		t.Fatalf("held=%q followed=%q", heldNames(held), heldNames(followed))
	}
}

// Following is a choice about a server the user already authorized: a project
// server still waiting for approval, or a record nobody can read, stays held.
func TestFollowingNeverOpensAnUnauthorizedOrUnreadableRecord(t *testing.T) {
	s := pinSpec(t, true)
	approvedBaseline(t, s)
	if err := SetToolPinsFollow(s, true); err != nil {
		t.Fatal(err)
	}
	unauthorized := s
	unauthorized.Authorized = false
	if held, followed := judgeToolPins(unauthorized, changedListing); len(held) == 0 || len(followed) != 0 {
		t.Fatalf("an unauthorized server was followed: held=%q followed=%q", heldNames(held), heldNames(followed))
	}
	if err := os.WriteFile(toolPinsPath(s), []byte(`{"version":1,"follow":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if held, followed := judgeToolPins(s, changedListing); len(followed) != 0 || len(held) != len(changedListing) {
		t.Fatalf("an unreadable record was followed: held=%q followed=%q", heldNames(held), heldNames(followed))
	}
	if err := SetToolPinsFollow(s, false); err == nil {
		t.Fatal("the choice overwrote a record it could not read")
	}
}

func TestFollowingAServerWithNoRecordYetRecordsEverythingAsListed(t *testing.T) {
	s := pinSpec(t, true)
	if err := SetToolPinsFollow(s, true); err != nil {
		t.Fatal(err)
	}
	if held, followed := judgeToolPins(s, changedListing); len(held) != 0 || len(followed) != 3 {
		t.Fatalf("held=%q followed=%q", heldNames(held), heldNames(followed))
	}
}

func TestAnAcceptanceKeepsTheFollowChoice(t *testing.T) {
	s := pinSpec(t, true)
	approvedBaseline(t, s)
	if err := SetToolPinsFollow(s, true); err != nil {
		t.Fatal(err)
	}
	pins, path, err := readToolPins(s)
	if err != nil || !pins.Follow {
		t.Fatalf("read: %v %v", pins, err)
	}
	if err := writeToolPins(path, pins.Tools, pins.Follow); err != nil || !ToolPinsFollow(s) {
		t.Fatalf("rewrite dropped the choice: %v", err)
	}
}
