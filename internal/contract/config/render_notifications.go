package config

import (
	"fmt"
	"strings"
)

// renderNotificationsSection writes [notifications] and its phone table.
func renderNotificationsSection(b *strings.Builder, c *Config) {
	b.WriteString("[notifications]\n")
	fmt.Fprintf(b, "enabled = %v   # system notifications for CLI and desktop turns; default off\n", c.Notifications.Enabled)
	fmt.Fprintf(b, "turn_done = %v   # notify when a turn finishes\n", c.Notifications.TurnDone)
	fmt.Fprintf(b, "approval_request = %v   # notify when a tool approval is waiting\n", c.Notifications.ApprovalRequest)
	fmt.Fprintf(b, "ask_request = %v   # notify when a question is waiting\n", c.Notifications.AskRequest)
	fmt.Fprintf(b, "feedback_reply = %v   # notify when a feedback report gets a reply\n", c.Notifications.FeedbackReply)
	fmt.Fprintf(b, "disk_low = %v   # notify when the workspace or state volume is nearly full\n", c.Notifications.DiskLow)
	fmt.Fprintf(b, "sound = %v   # play a short chime for the events above, with or without system notifications; default off\n", c.Notifications.Sound)
	b.WriteString("\n[notifications.phone]\n")
	fmt.Fprintf(b, "enabled = %v   # tell enrolled phones that something is waiting (a type and a count, never text); default off\n", c.Notifications.Phone.Enabled)
	fmt.Fprintf(b, "approval = %v   # a tool approval is waiting\n", c.Notifications.Phone.Approval)
	fmt.Fprintf(b, "input = %v   # the model asked a question\n", c.Notifications.Phone.Input)
	fmt.Fprintf(b, "error = %v   # a turn failed\n", c.Notifications.Phone.Error)
	b.WriteString("\n")
}
