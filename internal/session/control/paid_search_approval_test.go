package control

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
)

type paidAnswer struct {
	allow bool
	err   error
}

func paidSearchController(t *testing.T) (*Controller, chan event.Approval) {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	requests := make(chan event.Approval, 4)
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			requests <- e.Approval
		}
	})})
	return c, requests
}

func askPaid(c *Controller, backend, query string) chan paidAnswer {
	done := make(chan paidAnswer, 1)
	go func() {
		allow, err := paidSearchApprover{c}.ApprovePaidSearch(context.Background(), tool.PaidSearchRequest{
			Backend: backend, Query: query, Price: "$0.75 per 1,000 successful requests"})
		done <- paidAnswer{allow, err}
	}()
	return done
}

func nextApproval(t *testing.T, requests chan event.Approval) event.Approval {
	t.Helper()
	select {
	case a := <-requests:
		return a
	case <-time.After(30 * time.Second):
		t.Fatal("no approval was requested")
		return event.Approval{}
	}
}

// Only the user agrees to be charged: YOLO answers nothing here, the card names
// the price and shows the query that would leave the machine, and a session
// grant covers that backend and no other.
func TestPaidSearchApprovalWaitsForAPersonEvenInYolo(t *testing.T) {
	c, requests := paidSearchController(t)
	c.SetAutoApproveTools(true)
	done := askPaid(c, "deepseek_api", "golang context cancel")
	approval := nextApproval(t, requests)
	if approval.Tool != PaidSearchApprovalTool || approval.Subject != "deepseek_api" || !approval.Fresh {
		t.Fatalf("approval = %+v", approval)
	}
	for _, want := range []string{"$0.75 per 1,000 successful requests", "golang context cancel"} {
		if !strings.Contains(approval.Reason, want) {
			t.Fatalf("the card does not show %q: %q", want, approval.Reason)
		}
	}
	if !approval.AllowsSession || !approval.AllowsPersist {
		t.Fatalf("session=%v persist=%v, want both offered", approval.AllowsSession, approval.AllowsPersist)
	}
	select {
	case got := <-done:
		t.Fatalf("auto-approval answered a request to be charged: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
	c.Approve(approval.ID, true, true, false)
	if got := <-done; got.err != nil || !got.allow {
		t.Fatalf("approved = %+v", got)
	}
	if got := <-askPaid(c, "deepseek_api", "another query"); got.err != nil || !got.allow {
		t.Fatalf("the session grant did not cover the backend: %+v", got)
	}
	select {
	case a := <-requests:
		t.Fatalf("a granted backend was asked again: %+v", a)
	default:
	}
	other := askPaid(c, "other_paid", "q")
	a := nextApproval(t, requests)
	if a.Subject != "other_paid" {
		t.Fatalf("subject = %q", a.Subject)
	}
	c.Approve(a.ID, false, false, false)
	if got := <-other; got.err != nil || got.allow {
		t.Fatalf("declined = %+v", got)
	}
}

// "Allow once" covers one search: the next one asks again.
func TestPaidSearchAllowOnceDoesNotCarryOver(t *testing.T) {
	c, requests := paidSearchController(t)
	done := askPaid(c, "deepseek_api", "q1")
	c.Approve(nextApproval(t, requests).ID, true, false, false)
	if got := <-done; !got.allow {
		t.Fatal("allow once was refused")
	}
	again := askPaid(c, "deepseek_api", "q2")
	a := nextApproval(t, requests)
	c.Approve(a.ID, false, false, false)
	<-again
}

// "Always allow" writes the user's own file, so the next session does not ask.
func TestPaidSearchAlwaysAllowWritesTheUserSetting(t *testing.T) {
	c, requests := paidSearchController(t)
	done := askPaid(c, "deepseek_api", "q")
	a := nextApproval(t, requests)
	c.Approve(a.ID, true, false, true)
	if got := <-done; !got.allow {
		t.Fatal("always allow was refused")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		cfg := config.LoadForEdit(config.UserConfigPath())
		if len(cfg.Tools.WebSearchPaidAllowed) == 1 && cfg.Tools.WebSearchPaidAllowed[0] == "deepseek_api" {
			break
		}
		if time.Now().After(deadline) {
			raw, _ := os.ReadFile(config.UserConfigPath())
			t.Fatalf("the user config does not record the allowance:\n%s", raw)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The decision receipt names the backend, never the words the user searched for.
func TestPaidSearchDecisionReceiptCarriesNoQuery(t *testing.T) {
	if got := approvalDisplaySubject(PaidSearchApprovalTool, "deepseek_api", nil); got != "deepseek_api" {
		t.Fatalf("subject = %q", got)
	}
}

func askPaidFetch(c *Controller, backend, url string) chan paidAnswer {
	done := make(chan paidAnswer, 1)
	go func() {
		allow, err := paidSearchApprover{c}.ApprovePaidSearch(context.Background(), tool.PaidSearchRequest{
			Service: tool.PaidServiceFetch, Backend: backend, Query: url, Price: "$0.75 per 1,000 successful requests"})
		done <- paidAnswer{allow, err}
	}()
	return done
}

// Agreeing to be charged for searching is not agreeing to be charged for
// fetching, and the card for a fetch shows the address that would be sent.
func TestPaidFetchIsAskedApartFromPaidSearch(t *testing.T) {
	c, requests := paidSearchController(t)
	search := askPaid(c, "deepseek_api", "q")
	a := nextApproval(t, requests)
	c.Approve(a.ID, true, true, false)
	<-search
	fetch := askPaidFetch(c, "deepseek_api", "https://example.com/private-ish/page")
	card := nextApproval(t, requests)
	if card.Subject == a.Subject {
		t.Fatalf("a fetch rode the search grant: subject %q", card.Subject)
	}
	for _, want := range []string{"https://example.com/private-ish/page", "$0.75 per 1,000 successful requests", "fetch"} {
		if !strings.Contains(strings.ToLower(card.Reason), strings.ToLower(want)) {
			t.Fatalf("the fetch card lacks %q: %q", want, card.Reason)
		}
	}
	c.Approve(card.ID, true, true, false)
	if got := <-fetch; !got.allow {
		t.Fatalf("approved fetch = %+v", got)
	}
	if got := <-askPaidFetch(c, "deepseek_api", "https://other.example/"); !got.allow {
		t.Fatal("the fetch session grant did not cover the next fetch")
	}
}

// "Always allow" for a fetch records the fetch list, not the search one.
func TestPaidFetchAlwaysAllowWritesTheFetchSetting(t *testing.T) {
	c, requests := paidSearchController(t)
	done := askPaidFetch(c, "deepseek_api", "https://example.com")
	a := nextApproval(t, requests)
	c.Approve(a.ID, true, false, true)
	<-done
	deadline := time.Now().Add(5 * time.Second)
	for {
		cfg := config.LoadForEdit(config.UserConfigPath())
		if len(cfg.Tools.WebFetchPaidAllowed) == 1 && len(cfg.Tools.WebSearchPaidAllowed) == 0 {
			return
		}
		if time.Now().After(deadline) {
			raw, _ := os.ReadFile(config.UserConfigPath())
			t.Fatalf("the user config does not record the fetch allowance alone:\n%s", raw)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A frontend that reconnects while the card is open is shown the same card: the
// host's statement of what an answer may be saved as is part of it.
func TestReplayedPaidSearchCardStillOffersToSaveTheAnswer(t *testing.T) {
	c, requests := paidSearchController(t)
	done := askPaid(c, "deepseek_api", "golang context cancel")
	live := nextApproval(t, requests)
	c.ReplayPendingPromptsWith(func() event.Sink { return c.sink })
	replayed := nextApproval(t, requests)
	if replayed.AllowsPersist != live.AllowsPersist || replayed.AllowsSession != live.AllowsSession {
		t.Fatalf("replay says session=%v persist=%v, the live card said session=%v persist=%v",
			replayed.AllowsSession, replayed.AllowsPersist, live.AllowsSession, live.AllowsPersist)
	}
	c.Approve(live.ID, false, false, false)
	<-done
}
