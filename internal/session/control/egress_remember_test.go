package control

import (
	"context"
	"slices"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
)

func writeUserSandbox(t *testing.T, allowed, denied []string) {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	cfg := config.LoadForEdit(config.UserConfigPath())
	cfg.Sandbox.AllowedDomains, cfg.Sandbox.DeniedDomains = allowed, denied
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
}

func userAllowed(t *testing.T) []string {
	t.Helper()
	return config.LoadForEdit(config.UserConfigPath()).Sandbox.AllowedDomains
}

type egressAsk struct {
	approval event.Approval
	done     chan bool
	notices  chan event.Event
}

func askEgress(t *testing.T, c *Controller, requests chan event.Approval, host string) egressAsk {
	t.Helper()
	a := egressAsk{done: make(chan bool, 1), notices: make(chan event.Event, 4)}
	go func() {
		allow, _ := sandboxEscapeApprover{c}.ApproveEgress(context.Background(), host)
		a.done <- allow
	}()
	select {
	case a.approval = <-requests:
	case <-time.After(30 * time.Second):
		t.Fatal("egress approval was not requested")
	}
	return a
}

func newEgressController(t *testing.T) (*Controller, chan event.Approval, chan event.Event) {
	t.Helper()
	requests := make(chan event.Approval, 2)
	notices := make(chan event.Event, 8)
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		switch e.Kind {
		case event.ApprovalRequest:
			requests <- e.Approval
		case event.Notice:
			notices <- e
		}
	})})
	return c, requests, notices
}

// "Always allow" is offered only where it has a user list to join, and what it
// writes is exactly the host the card names.
func TestEgressCardOffersAlwaysAllowOnlyWhereTheUserHasAList(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed []string
		want    bool
	}{{"user list set", []string{"pypi.org"}, true}, {"no user list (a project's list may be in force)", nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			writeUserSandbox(t, tc.allowed, nil)
			c, requests, _ := newEgressController(t)
			ask := askEgress(t, c, requests, "files.example.org")
			if ask.approval.AllowsPersist != tc.want || !ask.approval.AllowsSession {
				t.Fatalf("card grants = session %v persist %v, want session true persist %v", ask.approval.AllowsSession, ask.approval.AllowsPersist, tc.want)
			}
			c.Approve(ask.approval.ID, false, false, false)
			<-ask.done
		})
	}
}

func TestEgressAlwaysAllowWritesTheHostToTheUserListAndGrantsTheSession(t *testing.T) {
	writeUserSandbox(t, []string{"pypi.org"}, nil)
	c, requests, notices := newEgressController(t)
	ask := askEgress(t, c, requests, "Files.Example.org")
	c.Approve(ask.approval.ID, true, false, true)
	if !<-ask.done {
		t.Fatal("the host was not allowed")
	}
	if got := userAllowed(t); !slices.Equal(got, []string{"pypi.org", "files.example.org"}) {
		t.Fatalf("allowed_domains = %v, want the new host appended in normal form", got)
	}
	saved := false
	for len(notices) > 0 {
		saved = saved || (<-notices).Code == event.NoticeCodePermissionSaved
	}
	if !saved {
		t.Fatal("the user was not told the host was written to their config")
	}
	if allow, err := (sandboxEscapeApprover{c}).ApproveEgress(context.Background(), "Files.Example.org"); err != nil || !allow {
		t.Fatalf("the host was asked about again in the same session: %v %v", allow, err)
	}
}

// A once or session answer never touches the file, and a decline with the
// "always" box ticked is still a decline.
func TestEgressAnswersOtherThanAlwaysLeaveTheFileAlone(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		allow, session, always bool
	}{{"once", true, false, false}, {"session", true, true, false}, {"declined despite always", false, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			writeUserSandbox(t, []string{"pypi.org"}, nil)
			c, requests, _ := newEgressController(t)
			ask := askEgress(t, c, requests, "files.example.org")
			c.Approve(ask.approval.ID, tc.allow, tc.session, tc.always)
			if got := <-ask.done; got != tc.allow {
				t.Fatalf("allowed = %v, want %v", got, tc.allow)
			}
			if got := userAllowed(t); !slices.Equal(got, []string{"pypi.org"}) {
				t.Fatalf("allowed_domains = %v, want it untouched", got)
			}
		})
	}
}

// The deny list outranks the allow list, so a host on it is not written even
// though the person said always; a host already covered is not duplicated.
func TestRememberEgressHostRespectsTheDenyListAndDoesNotDuplicate(t *testing.T) {
	writeUserSandbox(t, []string{"*.example.org"}, []string{"bad.example.org"})
	c := New(Options{})
	if r := c.rememberEgressHost("bad.example.org"); r.Err == nil || r.Saved {
		t.Fatalf("a denied host was written: %+v", r)
	}
	if r := c.rememberEgressHost("files.example.org"); r.Saved || r.CoveredBy == "" || r.Err != nil {
		t.Fatalf("a covered host: %+v, want reported as already covered", r)
	}
	if r := c.rememberEgressHost("  "); r.Err == nil {
		t.Fatal("a blank host was accepted")
	}
	if got := userAllowed(t); !slices.Equal(got, []string{"*.example.org"}) {
		t.Fatalf("allowed_domains = %v, want it untouched", got)
	}
	writeUserSandbox(t, nil, nil)
	if r := c.rememberEgressHost("files.example.org"); r.Err == nil || len(userAllowed(t)) != 0 {
		t.Fatalf("with no user list, one host became the whole list: %+v %v", r, userAllowed(t))
	}
}

// With the user's own file saying not to ask, a host outside the list passes
// without a card, is said once, and is remembered for the session.
func TestEgressUnlistedAllowSkipsTheCardAndSaysSo(t *testing.T) {
	writeUserSandbox(t, []string{"pypi.org"}, nil)
	cfg := config.LoadForEdit(config.UserConfigPath())
	cfg.Sandbox.EgressUnlisted = "allow"
	if err := cfg.SaveTo(config.UserConfigPath()); err != nil {
		t.Fatal(err)
	}
	c, requests, notices := newEgressController(t)
	for range 2 {
		allow, err := sandboxEscapeApprover{c}.ApproveEgress(context.Background(), "files.example.org")
		if err != nil || !allow {
			t.Fatalf("ApproveEgress = %v, %v; want allowed", allow, err)
		}
	}
	select {
	case a := <-requests:
		t.Fatalf("a card was shown: %+v", a)
	default:
	}
	if n := <-notices; n.Code != event.NoticeCodeEgressAutoAllowed || n.Detail != "files.example.org" {
		t.Fatalf("notice = %+v", n)
	}
	select {
	case n := <-notices:
		t.Fatalf("a second notice for the same host: %+v", n)
	default:
	}
}

func TestEgressUnlistedDefaultStillAsks(t *testing.T) {
	writeUserSandbox(t, []string{"pypi.org"}, nil)
	c, requests, _ := newEgressController(t)
	ask := askEgress(t, c, requests, "files.example.org")
	c.Approve(ask.approval.ID, false, false, false)
	if <-ask.done {
		t.Fatal("a declined card allowed the host")
	}
}
