package notify

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

// Occasions a phone is told about. They are the three kinds the accounts API
// accepts; nothing about the occasion beyond its kind and how many there were
// leaves this machine.
const (
	PhoneApproval = "approval"
	PhoneInput    = "input"
	PhoneError    = "error"
)

// PhoneSender delivers one report. The Studio host's cloud connection is the
// real one.
type PhoneSender interface {
	NotifyPhones(ctx context.Context, kind string, count int) error
}

// ErrNoPhoneLink: no sender has been attached yet, as before the account
// connection is up.
var ErrNoPhoneLink = errors.New("notify: no phone link attached")

// PhoneLink is a sender that can be attached after the sinks that will use it
// exist: the connection to the account is built later than the runtimes.
type PhoneLink struct{ held atomic.Pointer[PhoneSender] }

// Set attaches the sender every sink reading this link will use from now on.
func (l *PhoneLink) Set(s PhoneSender) {
	if l != nil {
		l.held.Store(&s)
	}
}

func (l *PhoneLink) NotifyPhones(ctx context.Context, kind string, count int) error {
	if l == nil {
		return ErrNoPhoneLink
	}
	held := l.held.Load()
	if held == nil || *held == nil {
		return ErrNoPhoneLink
	}
	return (*held).NotifyPhones(ctx, kind, count)
}

// phoneWindow is how long reports of one kind are folded after one is sent. The
// account service already replaces a waiting message of the same kind and limits
// how many a desktop may send, so a burst becomes one more report with a count
// rather than a flood.
const phoneWindow = 10 * time.Second

// phoneDeliveryTimeout bounds one report so a slow account service never holds
// a goroutine open.
const phoneDeliveryTimeout = 15 * time.Second

const maxPhoneCount = 99

// Phone turns the events a sink sees into reports to enrolled phones, under the
// desktop's switches. It reads the setting on every event, so flipping it takes
// effect on the next one.
type Phone struct {
	send  PhoneSender
	after func(time.Duration, func()) func() bool

	mu    sync.Mutex
	state map[string]*phoneKind
}

type phoneKind struct {
	open   bool
	folded int
}

// NewPhone reports through send. A nil sender reports nothing.
func NewPhone(send PhoneSender) *Phone {
	return &Phone{send: send, state: map[string]*phoneKind{}, after: func(d time.Duration, f func()) func() bool {
		t := time.AfterFunc(d, f)
		return t.Stop
	}}
}

// phoneKindOf names the occasion an event is, if it is one a phone can be told
// about: a tool approval waiting, a question waiting, or a turn that failed. A
// cancelled turn is not a failure (its error is nil).
func phoneKindOf(e event.Event) (string, bool) {
	switch e.Kind {
	case event.ApprovalRequest:
		return PhoneApproval, true
	case event.AskRequest:
		return PhoneInput, true
	case event.TurnDone:
		if e.Err != nil {
			return PhoneError, true
		}
	}
	return "", false
}

func phoneWanted(cfg config.PhoneNotificationsConfig, kind string) bool {
	if !cfg.Enabled {
		return false
	}
	switch kind {
	case PhoneApproval:
		return cfg.Approval
	case PhoneInput:
		return cfg.Input
	case PhoneError:
		return cfg.Error
	}
	return false
}

// Event reports e when the switches ask for it.
func (p *Phone) Event(cfg config.NotificationsConfig, e event.Event) {
	if p == nil || p.send == nil {
		return
	}
	kind, ok := phoneKindOf(e)
	if !ok || !phoneWanted(cfg.Phone, kind) {
		return
	}
	p.mu.Lock()
	st := p.state[kind]
	if st == nil {
		st = &phoneKind{}
		p.state[kind] = st
	}
	if st.open {
		st.folded++
		p.mu.Unlock()
		return
	}
	st.open = true
	p.mu.Unlock()
	go p.deliver(kind, 1)
	p.after(phoneWindow, func() { p.reopen(kind) })
}

// reopen ends one window. Reports folded during it go out as one with their
// count, which starts the next window; with none, the kind is idle again.
func (p *Phone) reopen(kind string) {
	p.mu.Lock()
	st := p.state[kind]
	n := st.folded
	st.folded = 0
	if n == 0 {
		st.open = false
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	go p.deliver(kind, min(n, maxPhoneCount))
	p.after(phoneWindow, func() { p.reopen(kind) })
}

func (p *Phone) deliver(kind string, count int) {
	ctx, cancel := context.WithTimeout(context.Background(), phoneDeliveryTimeout)
	defer cancel()
	if err := p.send.NotifyPhones(ctx, kind, count); err != nil && !errors.Is(err, ErrNoPhoneLink) {
		sendFailed(err)
	}
}
