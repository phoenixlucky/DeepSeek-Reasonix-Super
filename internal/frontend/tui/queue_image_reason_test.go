package tui

import (
	"strings"
	"testing"

	"reasonix/internal/base/i18n"
)

func TestQueueListSaysWhyAPicturedLineIsWaiting(t *testing.T) {
	snap := InboxSnapshot{Revision: 3, Items: []InboxItem{
		{ID: "aaaaaaaa1", Intent: "steer", State: "queued", Preview: "look at @shot.png", Images: true},
		{ID: "bbbbbbbb2", Intent: "steer", State: "queued", Preview: "words only"},
		{ID: "cccccccc3", Intent: "steer", State: "steer_accepted", Preview: "taken", Images: true},
	}}
	lines := strings.Split(renderQueueList(snap), "\n")
	reason := i18n.M.QueueImageWait
	if reason == "" {
		t.Fatal("the reason has no wording in the catalogue")
	}
	if !strings.Contains(lines[1], reason) {
		t.Fatalf("a queued line with a picture does not say why: %q", lines[1])
	}
	if strings.Contains(lines[2], reason) || strings.Contains(lines[3], reason) {
		t.Fatalf("the reason leaked onto a line it does not describe: %q / %q", lines[2], lines[3])
	}
}
