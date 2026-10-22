package control

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"reasonix/internal/base/i18n"
	"reasonix/internal/platform/feedback"
)

const feedbackCommandTimeout = 90 * time.Second

const feedbackUsage = `usage:
  /feedback <bug|idea|question|other> [--yes] <text>   (--yes only directly after the category)
  /feedback list                                       show your feedback and its status
  /feedback show <receipt>                             show the thread under one report
  /feedback reply <receipt> [--yes] <text>             answer the maintainers (--yes only directly after the receipt)
  /feedback withdraw <receipt> [--yes]                  take a report back (--yes directly after the receipt)
  /feedback link [--yes]                               show your reports on every device you link to your account
  /feedback unlink [--yes]                             stop linking this device
  /feedback links                                      list the devices linked to your account
  /feedback board [release]                            show the contribution board (all releases, or one release ranked)
  /feedback board join <name> [--yes] | leave | me    choose to be listed on it, or leave it
  /feedback name <nickname>                            set the nickname reports go out under`

const feedbackPublicNotice = "Your text and nickname become PUBLIC on GitHub (esengine/DeepSeek-Reasonix) only if we file your report as an issue; otherwise they stay private. You will get a receipt and, if it is filed, the link; we may also reply or ask questions here (/feedback list). Do not include secrets or private code. Security problems belong in SECURITY.md, not here."

func feedbackArgItems(prior []string) []SlashItem {
	if len(prior) > 1 {
		return nil
	}
	return []SlashItem{
		{Label: "bug", Insert: "bug", Hint: i18n.M.ArgFeedbackBug},
		{Label: "idea", Insert: "idea", Hint: i18n.M.ArgFeedbackIdea},
		{Label: "question", Insert: "question", Hint: i18n.M.ArgFeedbackQuestion},
		{Label: "other", Insert: "other", Hint: i18n.M.ArgFeedbackOther},
		{Label: "list", Insert: "list", Hint: i18n.M.ArgFeedbackList},
		{Label: "show", Insert: "show", Hint: i18n.M.ArgFeedbackShow},
		{Label: "reply", Insert: "reply", Hint: i18n.M.ArgFeedbackReply},
		{Label: "withdraw", Insert: "withdraw", Hint: i18n.M.ArgFeedbackWithdraw},
		{Label: "link", Insert: "link", Hint: i18n.M.ArgFeedbackLink},
		{Label: "unlink", Insert: "unlink", Hint: i18n.M.ArgFeedbackUnlink},
		{Label: "links", Insert: "links", Hint: i18n.M.ArgFeedbackLinks},
		{Label: "board", Insert: "board", Hint: i18n.M.ArgFeedbackBoard},
		{Label: "name", Insert: "name", Hint: i18n.M.ArgFeedbackName},
	}
}

// feedbackCommand answers a /feedback line as notices. The network part runs
// off the calling goroutine so a slow service never holds the input path.
func (c *Controller) feedbackCommand(args string) {
	if c.feedback.Service == nil || !c.feedback.Surface.Valid() {
		c.notice("feedback is not available in this session")
		return
	}
	verb, rest := args, ""
	if i := strings.IndexAny(args, " \t\r\n"); i >= 0 {
		verb, rest = args[:i], strings.TrimSpace(args[i:])
	}
	switch strings.ToLower(verb) {
	case "", "help":
		c.notice(feedbackUsage)
	case "list", "ls":
		go c.feedbackList()
	case "show":
		go c.feedbackShow(rest)
	case "reply":
		c.feedbackReply(rest)
	case "withdraw":
		c.feedbackWithdraw(rest)
	case "link":
		c.feedbackLinkCommand(rest, true)
	case "unlink":
		c.feedbackLinkCommand(rest, false)
	case "links":
		go c.feedbackLinks()
	case "board":
		c.feedbackBoardCommand(rest)
	case "name":
		if err := c.SetFeedbackDisplayName(rest); err != nil {
			c.notice(feedbackFailure(err))
			return
		}
		c.notice("nickname set to " + c.FeedbackDisplayName())
	default:
		c.feedbackSend(feedback.Category(strings.ToLower(verb)), rest)
	}
}

func (c *Controller) feedbackSend(category feedback.Category, text string) {
	confirmed := false
	body := text
	flag, after := text, ""
	if i := strings.IndexAny(text, " \t\r\n"); i >= 0 {
		flag, after = text[:i], text[i:]
	}
	if flag == "--yes" {
		confirmed, body = true, strings.TrimSpace(after)
	}
	name := c.FeedbackDisplayName()
	switch {
	case body == "":
		c.notice(feedbackUsage)
	case name == "":
		c.notice("set a nickname first (it is shown publicly): /feedback name <nickname>")
	case !confirmed:
		c.notice(feedbackPublicNotice + "\nNickname: " + name + "\nNothing was sent. To send it, run the same line again with --yes.")
	default:
		c.notice("sending feedback...")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
			defer cancel()
			got, err := c.SubmitFeedback(ctx, feedback.Draft{
				Category: category, Body: body, DisplayName: name,
				Env: feedback.EnvContext{Surface: c.feedback.Surface},
			})
			if err != nil {
				c.notice(feedbackFailure(err))
				return
			}
			msg := "Sent. Receipt " + got.Receipt + " - follow it with /feedback list."
			if got.Redacted {
				msg += "\nSecret-looking text was masked before sending."
			}
			c.notice(msg)
		}()
	}
}

func (c *Controller) feedbackList() {
	ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
	defer cancel()
	got, err := c.ListFeedback(ctx)
	if err != nil {
		c.notice(feedbackFailure(err))
		return
	}
	if len(got.Items) == 0 {
		c.notice("no feedback sent from this machine yet")
		return
	}
	var b strings.Builder
	if got.Offline {
		b.WriteString("(offline - showing what this machine remembers; statuses may be out of date)\n")
	}
	if up := got.Promotion; up != nil {
		fmt.Fprintf(&b, i18n.M.Feedback.LevelUpFmt+"\n", up.To, feedbackLevelName(up.To), up.From)
		if err := c.AckFeedbackPromotion(up.To); err != nil {
			b.WriteString("(could not remember that you were told: " + err.Error() + ")\n")
		}
	}
	if standing := feedbackStanding(got.Profile, got.Offline); standing != "" {
		b.WriteString(standing + "\n\n")
	}
	for _, it := range got.Items {
		fmt.Fprintf(&b, "%s  %-8s %-11s %s%s%s\n", it.Receipt, it.Category, feedbackStatusText(it), feedbackIssueRef(it), it.TitleSnippet, feedbackMarks(it))
	}
	if got.Unread > 0 {
		fmt.Fprintf(&b, "%d need your attention - open one with /feedback show <receipt>.\n", got.Unread)
	}
	c.notice(strings.TrimRight(b.String(), "\n"))
}

func feedbackMarks(it feedback.Item) string {
	var marks string
	if it.OtherInstall {
		marks += i18n.M.Feedback.OtherDeviceMark
	}
	if it.NeedsInput {
		marks += "  [needs your input]"
	}
	if it.UnreadReplies > 0 {
		marks += fmt.Sprintf("  [%d new]", it.UnreadReplies)
	}
	return marks
}

func (c *Controller) feedbackShow(receipt string) {
	if receipt == "" {
		c.notice(feedbackUsage)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
	defer cancel()
	got, err := c.ListFeedback(ctx)
	if err != nil {
		c.notice(feedbackFailure(err))
		return
	}
	i := slices.IndexFunc(got.Items, func(it feedback.Item) bool { return strings.EqualFold(it.Receipt, receipt) })
	if i < 0 {
		c.notice("no feedback with receipt " + receipt + " - see /feedback list")
		return
	}
	it := got.Items[i]
	var b strings.Builder
	if got.Offline {
		b.WriteString("(offline - showing what this machine remembers)\n")
	}
	fmt.Fprintf(&b, "%s  %s  %s\n%s\n", it.Receipt, it.Category, feedbackStatusText(it), it.TitleSnippet)
	if len(it.Replies) == 0 {
		b.WriteString("\nno replies yet\n")
	}
	for _, r := range it.Replies {
		who := "you"
		if r.Author == feedback.AuthorMaintainer {
			who = "maintainer"
		}
		fmt.Fprintf(&b, "\n%s, %s\n%s\n", who, r.CreatedAt.Local().Format("2006-01-02 15:04"), r.Body)
	}
	if it.NeedsInput {
		b.WriteString("\nThe maintainers are waiting for your answer: /feedback reply " + it.Receipt + " <text>\n")
	}
	var shown feedback.ReplyID
	for _, r := range it.Replies {
		shown = max(shown, r.ID)
	}
	if shown == 0 {
		c.notice(strings.TrimRight(b.String(), "\n"))
		return
	}
	if err := c.MarkFeedbackSeen(it.Receipt, shown); err != nil {
		b.WriteString("\n(could not remember that you read this: " + err.Error() + ")\n")
	}
	c.notice(strings.TrimRight(b.String(), "\n"))
}

func (c *Controller) feedbackReply(args string) {
	receipt, text := args, ""
	if i := strings.IndexAny(args, " \t\r\n"); i >= 0 {
		receipt, text = args[:i], strings.TrimSpace(args[i:])
	}
	confirmed, body := false, text
	flag, after := text, ""
	if i := strings.IndexAny(text, " \t\r\n"); i >= 0 {
		flag, after = text[:i], text[i:]
	}
	if flag == "--yes" {
		confirmed, body = true, strings.TrimSpace(after)
	}
	if receipt == "" || body == "" {
		c.notice(feedbackUsage)
		return
	}
	if !confirmed {
		msg := "Your reply goes to the maintainers."
		if it, ok := c.feedbackItem(receipt); ok && it.IssueNumber != nil {
			msg = fmt.Sprintf("This report has a public issue (#%d): your reply is also copied there, publicly.", *it.IssueNumber)
		}
		c.notice(msg + " Do not include secrets.\nNothing was sent. To send it, run the same line again with --yes directly after the receipt.")
		return
	}
	c.notice("sending reply...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
		defer cancel()
		if _, err := c.ReplyFeedback(ctx, receipt, body); err != nil {
			c.notice(feedbackReplyFailure(err, receipt))
			return
		}
		c.notice("Reply sent - follow it with /feedback show " + receipt + ".")
	}()
}

func (c *Controller) feedbackWithdraw(args string) {
	fields := strings.Fields(args)
	if len(fields) == 0 || len(fields) > 2 || (len(fields) == 2 && fields[1] != "--yes") {
		c.notice(feedbackUsage)
		return
	}
	receipt := fields[0]
	if len(fields) == 1 {
		msg := "Withdrawing takes this report back from the maintainers."
		if it, ok := c.feedbackItem(receipt); ok && it.IssueNumber != nil {
			msg = fmt.Sprintf("Withdrawing closes this report's public issue (#%d) and removes its images. The issue text already on GitHub stays.", *it.IssueNumber)
		}
		c.notice(msg + "\nNothing was sent. To withdraw it, run the same line again with --yes after the receipt.")
		return
	}
	c.notice("withdrawing...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
		defer cancel()
		if err := c.WithdrawFeedback(ctx, receipt); err != nil {
			c.notice(feedbackWithdrawFailure(err, receipt))
			return
		}
		c.notice("Withdrawn - " + receipt + " is closed.")
	}()
}

// feedbackWithdrawFailure speaks for a withdrawal. The service treats a repeat
// as done, so an unanswered request is safe to send again.
func feedbackWithdrawFailure(err error, receipt string) string {
	switch {
	case errors.Is(err, feedback.ErrNotWithdrawable):
		return receipt + " can no longer be withdrawn: it already has an outcome, or this install cannot speak for it"
	case errors.Is(err, feedback.ErrNotFound):
		return "no such report for this install: " + receipt
	case errors.Is(err, feedback.ErrOffline), errors.Is(err, feedback.ErrUnavailable):
		return "the withdrawal may not have gone through - run it again to be sure"
	}
	return feedbackFailure(err)
}

func (c *Controller) feedbackItem(receipt string) (feedback.Item, bool) {
	return c.feedback.Service.Item(receipt)
}

func feedbackIssueRef(it feedback.Item) string {
	if it.IssueNumber == nil {
		return ""
	}
	return fmt.Sprintf("#%d ", *it.IssueNumber)
}

func feedbackStatusText(it feedback.Item) string {
	switch it.Status {
	case feedback.StatusFixed:
		if it.ResolvedVersion == "next" || it.ResolvedVersion == "" {
			return "fixed (next version)"
		}
		return "fixed in " + it.ResolvedVersion
	case feedback.StatusDuplicate:
		if it.DuplicateOf != nil {
			return fmt.Sprintf("duplicate of #%d", *it.DuplicateOf)
		}
		return "duplicate"
	case feedback.StatusWontFix:
		return "won't fix"
	case feedback.StatusInProgress:
		return "in progress"
	case feedback.StatusNeedsInfo:
		return "needs info"
	}
	return string(it.Status)
}

func feedbackFailure(err error) string {
	var invalid *feedback.InvalidError
	if said, ok := feedbackRefusal(err); ok {
		return said
	}
	switch {
	case errors.As(err, &invalid):
		return "feedback not sent: " + invalid.Field + " is not acceptable (" + invalid.Reason + ")"
	case errors.Is(err, feedback.ErrTooLarge):
		return "feedback not sent: it is too large - shorten the text"
	case errors.Is(err, feedback.ErrRateLimited):
		if after := feedback.RetryAfter(err); after > 0 {
			return "feedback not sent: too many submissions, try again in " + after.Round(time.Second).String()
		}
		return "feedback not sent: too many submissions, try again later"
	case errors.Is(err, feedback.ErrDisabled):
		return "feedback is switched off right now; open an issue on GitHub instead"
	case errors.Is(err, feedback.ErrBusy):
		return "feedback is not being accepted right now (the service is at its daily capacity) - try again tomorrow"
	case errors.Is(err, feedback.ErrReplyLimit):
		return "reply not sent: this report has reached its reply limit, or you replied too often - try again later"
	case errors.Is(err, feedback.ErrNotReplyable):
		return "reply not sent: this report takes no reply right now"
	case errors.Is(err, feedback.ErrChallengeRequired):
		return "feedback not sent: the service asks for a verification step this terminal cannot show - use Studio instead"
	case errors.Is(err, feedback.ErrImageMetadata):
		return "feedback not sent: an image could not be cleaned of its metadata"
	case errors.Is(err, feedback.ErrDuplicate):
		return "the same feedback was just sent"
	case errors.Is(err, feedback.ErrBadToken):
		return "the feedback service did not accept this install's identity - run the command again"
	case errors.Is(err, feedback.ErrOffline):
		return "cannot reach the feedback service - check the network and run the command again"
	case errors.Is(err, feedback.ErrUnavailable):
		return "the feedback service failed - try again later"
	}
	return "feedback: " + err.Error()
}

// feedbackReplyFailure speaks for a reply, which carries no idempotency key: an
// unanswered request may already have landed, so it is never told to simply retry.
func feedbackReplyFailure(err error, receipt string) string {
	switch {
	case errors.Is(err, feedback.ErrOffline), errors.Is(err, feedback.ErrUnavailable):
		return "the reply may or may not have been sent - check /feedback show " + receipt + " before sending it again"
	case errors.Is(err, feedback.ErrBadToken):
		return "reply not sent: this install's feedback identity is no longer accepted, so this report cannot be answered from here - send a new report instead"
	case errors.Is(err, feedback.ErrDuplicate):
		return "a reply to this report is already being sent"
	}
	return feedbackFailure(err)
}
