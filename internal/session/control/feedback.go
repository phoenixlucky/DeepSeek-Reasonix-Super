package control

import (
	"context"

	"reasonix/internal/platform/feedback"
)

// Feedback covers sending the maintainers a report and following what becomes
// of it. The service is install-scoped, not session-scoped: every controller a
// host builds reaches the same identity and receipts on disk.
type Feedback interface {
	FeedbackLimits() feedback.Limits
	FeedbackEnv(surface feedback.Surface, locale string) feedback.Env
	FeedbackDisplayName() string
	SetFeedbackDisplayName(name string) error
	SubmitFeedback(ctx context.Context, d feedback.Draft) (feedback.Receipt, error)
	ListFeedback(ctx context.Context) (feedback.Mine, error)
	ReplyFeedback(ctx context.Context, receipt, body string, images ...feedback.Image) (feedback.ReplyReceipt, error)
	MarkFeedbackSeen(receipt string, upTo feedback.ReplyID) error
	AckFeedbackPromotion(level int) error
	WithdrawFeedback(ctx context.Context, receipt string) error
	LinkFeedbackAccount(ctx context.Context) error
	UnlinkFeedbackAccount(ctx context.Context) error
	UnlinkFeedbackInstall(ctx context.Context, shortID string) error
	FeedbackLinks(ctx context.Context) ([]feedback.LinkedInstall, error)
	FeedbackBoard(ctx context.Context, release string) (feedback.Board, error)
	FeedbackBoardMe(ctx context.Context) (feedback.BoardMe, error)
	JoinFeedbackBoard(ctx context.Context, displayName string) (feedback.BoardMe, error)
	LeaveFeedbackBoard(ctx context.Context) (feedback.BoardMe, error)
}

// FeedbackOptions is what assembly hands the controller for Feedback: the
// install's report client, the active provider's kind as a report states it, and
// the frontend a report names (empty leaves /feedback unavailable). A nil
// service answers feedback.ErrDisabled, so a controller built without one
// refuses rather than pretending to send.
type FeedbackOptions struct {
	Service      *feedback.Service
	ProviderKind string
	Surface      feedback.Surface
}

func (c *Controller) FeedbackLimits() feedback.Limits { return feedback.DefaultLimits }

func (c *Controller) FeedbackEnv(surface feedback.Surface, locale string) feedback.Env {
	return feedback.CollectEnv(feedback.EnvContext{Surface: surface, Locale: locale, ProviderKind: c.feedback.ProviderKind})
}

func (c *Controller) FeedbackDisplayName() string {
	if c.feedback.Service == nil {
		return ""
	}
	return c.feedback.Service.DisplayName()
}

func (c *Controller) SetFeedbackDisplayName(name string) error {
	if c.feedback.Service == nil {
		return feedback.ErrDisabled
	}
	return c.feedback.Service.SetDisplayName(name)
}

func (c *Controller) SubmitFeedback(ctx context.Context, d feedback.Draft) (feedback.Receipt, error) {
	if c.feedback.Service == nil {
		return feedback.Receipt{}, feedback.ErrDisabled
	}
	d.Env.ProviderKind = c.feedback.ProviderKind
	return c.feedback.Service.Submit(ctx, d)
}

func (c *Controller) ListFeedback(ctx context.Context) (feedback.Mine, error) {
	if c.feedback.Service == nil {
		return feedback.Mine{}, feedback.ErrDisabled
	}
	got, err := c.feedback.Service.ListMine(ctx)
	if err == nil && got.Promotion != nil && !c.FeedbackLevelNotices().Enabled {
		// Nothing is said, but the baseline follows the level: switching the
		// notices on later never announces a promotion that already happened.
		if aerr := c.feedback.Service.AckPromotion(got.Promotion.To); aerr != nil {
			return got, aerr
		}
		got.Promotion = nil
	}
	return got, err
}

// AckFeedbackPromotion records that the person was told about level.
func (c *Controller) AckFeedbackPromotion(level int) error {
	if c.feedback.Service == nil {
		return feedback.ErrDisabled
	}
	return c.feedback.Service.AckPromotion(level)
}

func (c *Controller) ReplyFeedback(ctx context.Context, receipt, body string, images ...feedback.Image) (feedback.ReplyReceipt, error) {
	if c.feedback.Service == nil {
		return feedback.ReplyReceipt{}, feedback.ErrDisabled
	}
	return c.feedback.Service.Reply(ctx, receipt, body, images...)
}

// WithdrawFeedback takes one of this install's reports back. The service decides
// whether it still can; every frontend asks through here.
func (c *Controller) WithdrawFeedback(ctx context.Context, receipt string) error {
	if c.feedback.Service == nil {
		return feedback.ErrDisabled
	}
	return c.feedback.Service.Withdraw(ctx, receipt)
}

func (c *Controller) MarkFeedbackSeen(receipt string, upTo feedback.ReplyID) error {
	if c.feedback.Service == nil {
		return feedback.ErrDisabled
	}
	return c.feedback.Service.MarkSeen(receipt, upTo)
}

// LinkFeedbackAccount ties this install to the signed-in account. It is only
// ever the answer to a person asking: nothing links at sign-in.
func (c *Controller) LinkFeedbackAccount(ctx context.Context) error {
	if c.feedback.Service == nil {
		return feedback.ErrDisabled
	}
	return c.feedback.Service.Link(ctx)
}

func (c *Controller) UnlinkFeedbackAccount(ctx context.Context) error {
	if c.feedback.Service == nil {
		return feedback.ErrDisabled
	}
	return c.feedback.Service.Unlink(ctx)
}

func (c *Controller) UnlinkFeedbackInstall(ctx context.Context, shortID string) error {
	if c.feedback.Service == nil {
		return feedback.ErrDisabled
	}
	return c.feedback.Service.UnlinkInstall(ctx, shortID)
}

func (c *Controller) FeedbackLinks(ctx context.Context) ([]feedback.LinkedInstall, error) {
	if c.feedback.Service == nil {
		return nil, feedback.ErrDisabled
	}
	return c.feedback.Service.Links(ctx)
}

// FeedbackBoard reads the public contribution board, all releases or one.
func (c *Controller) FeedbackBoard(ctx context.Context, release string) (feedback.Board, error) {
	if c.feedback.Service == nil {
		return feedback.Board{}, feedback.ErrDisabled
	}
	return c.feedback.Service.Board(ctx, release)
}

// FeedbackBoardMe reads the signed-in account's own choice about the board.
func (c *Controller) FeedbackBoardMe(ctx context.Context) (feedback.BoardMe, error) {
	if c.feedback.Service == nil {
		return feedback.BoardMe{}, feedback.ErrDisabled
	}
	return c.feedback.Service.BoardMe(ctx)
}

// JoinFeedbackBoard puts the account on the board under the name the person gave.
// It is only ever the answer to a person asking, and the service decides the name.
func (c *Controller) JoinFeedbackBoard(ctx context.Context, displayName string) (feedback.BoardMe, error) {
	if c.feedback.Service == nil {
		return feedback.BoardMe{}, feedback.ErrDisabled
	}
	return c.feedback.Service.JoinBoard(ctx, displayName)
}

// LeaveFeedbackBoard takes the account off the board.
func (c *Controller) LeaveFeedbackBoard(ctx context.Context) (feedback.BoardMe, error) {
	if c.feedback.Service == nil {
		return feedback.BoardMe{}, feedback.ErrDisabled
	}
	return c.feedback.Service.LeaveBoard(ctx)
}
