package control

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"reasonix/internal/base/i18n"
	"reasonix/internal/platform/feedback"
)

// feedbackLinkCommand runs /feedback link and /feedback unlink: both change who
// can see this device's reports, so each says what it does and sends nothing
// until --yes follows the verb.
func (c *Controller) feedbackLinkCommand(args string, link bool) {
	fields := strings.Fields(args)
	if len(fields) > 1 || (len(fields) == 1 && fields[0] != "--yes") {
		c.notice(feedbackUsage)
		return
	}
	m := i18n.M.Feedback
	if len(fields) == 0 {
		c.notice(map[bool]string{true: m.LinkExplain, false: m.UnlinkExplain}[link])
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
		defer cancel()
		var err error
		if link {
			err = c.LinkFeedbackAccount(ctx)
		} else {
			err = c.UnlinkFeedbackAccount(ctx)
		}
		if err != nil {
			c.notice(feedbackLinkFailure(err))
			return
		}
		if link {
			c.notice(m.LinkDone + "\n" + m.LinkBoardHint)
			return
		}
		c.notice(m.UnlinkDone)
	}()
}

func (c *Controller) feedbackLinks() {
	ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
	defer cancel()
	installs, err := c.FeedbackLinks(ctx)
	if err != nil {
		c.notice(feedbackLinkFailure(err))
		return
	}
	m := i18n.M.Feedback
	if len(installs) == 0 {
		c.notice(m.LinksNone)
		return
	}
	var b strings.Builder
	b.WriteString(m.LinksHeader + "\n")
	for _, in := range installs {
		mark := ""
		if in.Current {
			mark = m.ThisDevice
		}
		fmt.Fprintf(&b, m.LinksRowFmt+"\n", in.ID, mark, in.LinkedAt.Local().Format("2006-01-02"))
	}
	c.notice(strings.TrimRight(b.String(), "\n"))
}

// feedbackLinkFailure speaks from the refusal's identity; anything else is the
// ordinary feedback failure.
func feedbackLinkFailure(err error) string {
	m := i18n.M.Feedback
	switch {
	case errors.Is(err, feedback.ErrAccountRequired):
		return m.NeedSignIn
	case errors.Is(err, feedback.ErrLinkConflict):
		return m.LinkConflict
	case errors.Is(err, feedback.ErrLinkLimit):
		return m.LinkLimit
	case errors.Is(err, feedback.ErrNoInstall):
		return m.NoInstall
	}
	return feedbackFailure(err)
}
