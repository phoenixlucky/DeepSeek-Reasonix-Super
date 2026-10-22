package control

import (
	"strings"
	"testing"
	"time"
)

var sessionOptions = []SessionOption{
	{ID: "20261018-020806-test", Title: "Rotate the signing key", At: time.Date(2026, 10, 18, 2, 8, 0, 0, time.UTC), Turns: 4},
	{ID: "20261017-101010-test", Title: "Fix the flaky branch menu", At: time.Date(2026, 10, 17, 10, 10, 0, 0, time.UTC), Turns: 9},
}

func TestActiveSessionTokenFindsAHashWordAtTheCaretAndNothingElse(t *testing.T) {
	cases := []struct {
		line   string
		cursor int
		from   int
		to     int
		query  string
		ok     bool
	}{
		{"#", 1, 0, 1, "", true},
		{"look at #sig", 12, 8, 12, "sig", true},
		{"look at #sig and more", 12, 8, 12, "sig", true},
		{"look at #si|g and more", 11, 8, 12, "si", true},
		{"mail me@host#tag", 16, 0, 0, "", false},
		{"already done #key then", 22, 0, 0, "", false},
		{"two\n#x", 6, 4, 6, "x", true},
		{"# heading", 9, 0, 0, "", false},
	}
	for _, c := range cases {
		line := strings.ReplaceAll(c.line, "|", "")
		from, to, query, ok := ActiveSessionToken(line, min(c.cursor, len(line)))
		if ok != c.ok || (ok && (from != c.from || to != c.to || query != c.query)) {
			t.Errorf("%q @%d = (%d,%d,%q,%v), want (%d,%d,%q,%v)", c.line, c.cursor, from, to, query, ok, c.from, c.to, c.query, c.ok)
		}
	}
}

func TestSessionRefItemsFilterOnWhatWasSaidAboutNotOnTheIDAndKeepRecentFirst(t *testing.T) {
	all := SessionRefItems(sessionOptions, "")
	if len(all) != 2 || all[0].Insert != "@session:20261018-020806-test" || all[0].Kind != "session" || all[0].Label != "Rotate the signing key" {
		t.Fatalf("empty query = %+v", all)
	}
	if got := SessionRefItems(sessionOptions, "FLAKY"); len(got) != 1 || got[0].Insert != "@session:20261017-101010-test" {
		t.Fatalf("a title match is not case sensitive: %+v", got)
	}
	if got := SessionRefItems(sessionOptions, "2026"); len(got) != 0 {
		t.Fatalf("an id fragment must not open a menu, or an issue number would hijack Enter: %+v", got)
	}
}

func TestCompleteOffersThePastConversationsForAHashWord(t *testing.T) {
	line := "continue from #sign"
	c := Complete(line, len(line), CompletionData{Sessions: sessionOptions})
	if c.Kind != CompleteSession || c.From != 14 || c.To != len(line) || len(c.Items) != 1 {
		t.Fatalf("completion = %+v", c)
	}
	if got := Complete("continue from #nothing-like-it", 30, CompletionData{Sessions: sessionOptions}); len(got.Items) != 0 {
		t.Fatalf("no title matches, yet %+v", got)
	}
}
