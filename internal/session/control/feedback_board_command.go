package control

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/base/i18n"
	"reasonix/internal/platform/feedback"
)

// feedbackBoardCommand runs /feedback board: the public list, and the person's own
// choice to be on it. Joining sends a name that becomes public, so it says what
// it would show and sends nothing until --yes follows the name.
func (c *Controller) feedbackBoardCommand(args string) {
	fields := strings.Fields(args)
	m := i18n.M.Feedback
	switch {
	case len(fields) == 0:
		go c.feedbackBoardShow("")
	case fields[0] == "leave" && len(fields) == 1:
		go c.feedbackBoardChange(func(ctx context.Context) (feedback.BoardMe, error) { return c.LeaveFeedbackBoard(ctx) }, m.BoardLeaveDone)
	case fields[0] == "me" && len(fields) == 1:
		go c.feedbackBoardMe()
	case fields[0] == "join":
		c.feedbackBoardJoin(fields[1:])
	case len(fields) == 1:
		go c.feedbackBoardShow(fields[0])
	default:
		c.notice(feedbackUsage)
	}
}

func (c *Controller) feedbackBoardShow(release string) {
	ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
	defer cancel()
	board, err := c.FeedbackBoard(ctx, release)
	if err != nil {
		c.notice(feedbackBoardFailure(err))
		return
	}
	m := i18n.M.Feedback
	if !board.Available {
		c.notice(m.BoardOff)
		return
	}
	view := m.BoardAllView
	if board.Release != nil {
		view = fmt.Sprintf(m.BoardReleaseFmt, *board.Release)
	}
	if len(board.Entries) == 0 {
		c.notice(fmt.Sprintf(m.BoardHeaderFmt, view) + "\n" + m.BoardNone)
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(m.BoardHeaderFmt, view) + "\n")
	for _, e := range board.Entries {
		place := ""
		if e.Rank != nil {
			place = fmt.Sprintf("%d.", *e.Rank)
		}
		where := ""
		if board.Release == nil {
			var parts []string
			for _, r := range e.Releases {
				parts = append(parts, fmt.Sprintf("%s x%d", r.Version, r.Count))
			}
			if len(parts) > 0 {
				where = fmt.Sprintf(m.BoardRowReleasesFmt, strings.Join(parts, ", "))
			}
		}
		fmt.Fprintf(&b, m.BoardRowFmt+"\n", place, e.DisplayName, e.Credits, where)
	}
	c.notice(strings.TrimRight(b.String(), "\n"))
}

func (c *Controller) feedbackBoardMe() {
	ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
	defer cancel()
	me, err := c.FeedbackBoardMe(ctx)
	if err != nil {
		c.notice(feedbackBoardFailure(err))
		return
	}
	m := i18n.M.Feedback
	switch {
	case me.Delisted:
		c.notice(m.BoardDelisted)
	case me.Listed && me.DisplayName != nil:
		c.notice(fmt.Sprintf(m.BoardMeListedFmt, *me.DisplayName))
	default:
		c.notice(m.BoardMeNot)
	}
}

func (c *Controller) feedbackBoardJoin(rest []string) {
	m := i18n.M.Feedback
	confirmed := false
	if len(rest) > 0 && rest[len(rest)-1] == "--yes" {
		confirmed, rest = true, rest[:len(rest)-1]
	}
	name := strings.Join(rest, " ")
	if name == "" {
		c.notice(feedbackUsage)
		return
	}
	if !confirmed {
		c.notice(fmt.Sprintf(m.BoardJoinExplainFmt, name))
		return
	}
	go c.feedbackBoardChange(func(ctx context.Context) (feedback.BoardMe, error) { return c.JoinFeedbackBoard(ctx, name) }, m.BoardJoinDone)
}

func (c *Controller) feedbackBoardChange(act func(context.Context) (feedback.BoardMe, error), done string) {
	ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
	defer cancel()
	if _, err := act(ctx); err != nil {
		c.notice(feedbackBoardFailure(err))
		return
	}
	c.notice(done)
}

// feedbackBoardFailure speaks from the refusal's identity; anything else is the
// ordinary account-link failure.
func feedbackBoardFailure(err error) string {
	m := i18n.M.Feedback
	switch {
	case errors.Is(err, feedback.ErrBoardNameInvalid):
		return m.BoardNameInvalid
	case errors.Is(err, feedback.ErrBoardNameReserved):
		return m.BoardNameReserved
	case errors.Is(err, feedback.ErrBoardNameTaken):
		return m.BoardNameTaken
	case errors.Is(err, feedback.ErrBoardDelisted):
		return m.BoardDelisted
	case errors.Is(err, feedback.ErrBoardNoLink):
		return m.BoardNoLink
	}
	return feedbackLinkFailure(err)
}
