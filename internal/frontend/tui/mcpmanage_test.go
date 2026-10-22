package tui

import (
	"maps"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const mcpOne = `{"name":"docs","state":"ready","enabled":true,"transport":"http","source":"user","tools":2}`

func mcpServers(rows ...string) map[string]any {
	var servers []any
	for _, r := range rows {
		servers = append(servers, rawJSON(r))
	}
	return map[string]any{"servers": servers}
}

type rawJSON string

func (r rawJSON) MarshalJSON() ([]byte, error) { return []byte(r), nil }

const parsedDraft = `{"servers":[{"name":"acme","transport":"stdio","command":"npx","args":["-y","@acme/mcp","--token","s3cret"],"env":{"KEY":"v"},
 "displayCommand":"npx","displayArgs":["-y","@acme/mcp","--token","***"],"displayEnv":{"KEY":"***"}}],
 "risks":[{"server":"acme","kind":"launcher","field":"command","detail":"runs a package fetched at launch"}]}`

func typeInto(m *model, text string) {
	m.Update(tea.KeyPressMsg{Code: rune(text[0]), Text: text})
}

func mcpAddRig(t *testing.T, extra map[string]any) (*model, *queueKernel) {
	answers := map[string]any{
		"GET /mcp":          mcpServers(mcpOne),
		"POST /mcp/parse":   rawJSON(parsedDraft),
		"POST /mcp/install": map[string]any{"name": "acme", "state": "ready", "toolCount": 3, "action": "none"},
		"POST /mcp/remove":  map[string]any{"name": "docs", "disconnected": true, "stillConfigured": false},
	}
	maps.Copy(answers, extra)
	prev := mcpReviewDelay
	mcpReviewDelay = 0
	t.Cleanup(func() { mcpReviewDelay = prev })
	m, k := queueModel(t, answers)
	queueSend(m, "/mcp")
	return m, k
}

func startAdd(m *model, line string) {
	run(m, keyRune(m, 'n'))
	typeInto(m, line)
	run(m, press(m, "enter"))
}

func TestMCPEmptyListStillOpensThePanelWithTheHandHelp(t *testing.T) {
	m, k := queueModel(t, map[string]any{"GET /mcp": map[string]any{"servers": []any{}}})
	queueSend(m, "/mcp")
	if m.mcp == nil {
		t.Fatalf("the panel did not open on an empty list; calls:\n%s", k.seen())
	}
	v := m.View().Content
	if !strings.Contains(v, "reasonix.toml") || !strings.Contains(v, "[[plugins]]") {
		t.Fatalf("the by-hand help is gone:\n%s", v)
	}
}

func TestMCPAddShowsWhatWouldRunAndInstallsNothingUntilConfirmed(t *testing.T) {
	m, k := mcpAddRig(t, nil)
	startAdd(m, "npx -y @acme/mcp --token s3cret")
	calls := k.seen()
	if !strings.Contains(calls, `POST /mcp/parse {"input":"npx -y @acme/mcp --token s3cret"}`) {
		t.Fatalf("parse was not asked:\n%s", calls)
	}
	v := m.View().Content
	for _, want := range []string{"acme", "npx -y @acme/mcp --token ***", "runs a package fetched at launch"} {
		if !strings.Contains(v, want) {
			t.Fatalf("review lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "s3cret") {
		t.Fatalf("the review shows a secret:\n%s", v)
	}
	if strings.Contains(calls, "/mcp/install") {
		t.Fatalf("installed before the person said so:\n%s", calls)
	}
	run(m, press(m, "esc"))
	if strings.Contains(k.seen(), "/mcp/install") {
		t.Fatal("cancel installed")
	}
}

func TestMCPAddInstallsAtTheChosenScopeSendingTheRealValues(t *testing.T) {
	for key, scope := range map[rune]string{'y': "user", 'l': "local", 'p': "project"} {
		m, k := mcpAddRig(t, nil)
		startAdd(m, "npx -y @acme/mcp --token s3cret")
		run(m, keyRune(m, key))
		calls := k.seen()
		if !strings.Contains(calls, `"scope":"`+scope+`"`) || !strings.Contains(calls, `"s3cret"`) {
			t.Fatalf("%c: install did not carry scope %s and the real args:\n%s", key, scope, calls)
		}
		if got := queueLastNotice(m); !strings.Contains(got, "acme") || !strings.Contains(got, "3") {
			t.Fatalf("%c: notice = %q", key, got)
		}
	}
}

func TestMCPAddSaysWhatIsLeftToDoWhenTheServerNeedsAction(t *testing.T) {
	m, _ := mcpAddRig(t, map[string]any{"POST /mcp/install": map[string]any{"name": "acme", "state": "action_required", "toolCount": 0, "action": "authenticate", "message": "sign in to acme"}})
	startAdd(m, "https://acme.example/mcp")
	run(m, keyRune(m, 'y'))
	if got := queueLastNotice(m); !strings.Contains(got, "sign in to acme") {
		t.Fatalf("notice = %q", got)
	}
}

func TestMCPAddThatCannotBeReadGoesBackToTheListWithTheReason(t *testing.T) {
	m, k := queueModel(t, map[string]any{"GET /mcp": mcpServers(mcpOne)})
	queueSend(m, "/mcp")
	startAdd(m, "not a server")
	// the stub answers 204 with no body for the unknown parse route
	if m.mcp == nil || m.mcp.review != nil {
		t.Fatalf("a failed parse left a review open: %+v", m.mcp)
	}
	if strings.Contains(k.seen(), "/mcp/install") {
		t.Fatal("install was reached without a review")
	}
}

func TestMCPRemoveAsksFirstAndThenReadsTheListAgain(t *testing.T) {
	m, k := mcpAddRig(t, nil)
	run(m, keyRune(m, 'd'))
	if v := m.View().Content; !strings.Contains(v, "docs") || !strings.Contains(v, "y/n") {
		t.Fatalf("no confirmation:\n%s", v)
	}
	run(m, keyRune(m, 'n'))
	if strings.Contains(k.seen(), "/mcp/remove") {
		t.Fatal("a declined removal reached the kernel")
	}
	run(m, keyRune(m, 'd'))
	run(m, keyRune(m, 'y'))
	calls := k.seen()
	if !strings.Contains(calls, `POST /mcp/remove {"name":"docs"}`) || strings.Count(calls, "GET /mcp") < 2 {
		t.Fatalf("remove or the re-read is missing:\n%s", calls)
	}
}

func draftOf(args []string, url string, env map[string]string) string {
	quote := func(v string) string { return `"` + v + `"` }
	var a []string
	for _, x := range args {
		a = append(a, quote(x))
	}
	var e []string
	for k, v := range env {
		e = append(e, quote(k)+":"+quote(v))
	}
	server := `{"name":"acme","transport":"stdio","command":"npx","args":[` + strings.Join(a, ",") + `],"displayCommand":"npx","displayArgs":[` + strings.Join(a, ",") + `],"displayEnv":{` + strings.Join(e, ",") + `}`
	if url != "" {
		server = `{"name":"acme","transport":"http","url":` + quote(url) + `,"displayUrl":` + quote(url) + `,"displayHeaders":{` + strings.Join(e, ",") + `}`
	}
	return `{"servers":[` + server + `}],"risks":[]}`
}

func flat(m *model) string {
	return strings.Join(strings.Fields(ansi.Strip(m.View().Content)), "")
}

func TestMCPReviewShowsEveryCharacterOfALongCommandOnANarrowTerminal(t *testing.T) {
	long := strings.Repeat("abcdefghij", 40)
	env := strings.Repeat("0123456789", 12)
	m, _ := mcpAddRig(t, map[string]any{"POST /mcp/parse": rawJSON(draftOf([]string{"-y", long, "--tail", "THE-LAST-ARG"}, "", map[string]string{"KEY": env}))})
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 60})
	startAdd(m, "anything")
	got := flat(m)
	for _, want := range []string{long, "THE-LAST-ARG", env} {
		if !strings.Contains(got, want) {
			t.Fatalf("the review cut off %q:\n%s", want, ansi.Strip(m.View().Content))
		}
	}
	m2, _ := mcpAddRig(t, map[string]any{"POST /mcp/parse": rawJSON(draftOf(nil, "https://example.com/"+long+"/END", map[string]string{"Authorization": env}))})
	m2.Update(tea.WindowSizeMsg{Width: 40, Height: 60})
	startAdd(m2, "anything")
	if got := flat(m2); !strings.Contains(got, long+"/END") || !strings.Contains(got, env) {
		t.Fatalf("the address or header was cut off:\n%s", ansi.Strip(m2.View().Content))
	}
}

func TestMCPReviewTooLongToReviewOffersNoInstall(t *testing.T) {
	m, k := mcpAddRig(t, map[string]any{"POST /mcp/parse": rawJSON(draftOf([]string{strings.Repeat("x", 5000)}, "", nil))})
	startAdd(m, "anything")
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "too long") {
		t.Fatalf("no explanation:\n%s", v)
	}
	for _, key := range []rune{'y', 'l', 'p'} {
		run(m, keyRune(m, key))
	}
	if strings.Contains(k.seen(), "/mcp/install") {
		t.Fatalf("a command nobody could read in full was installed:\n%s", k.seen())
	}
	run(m, press(m, "esc"))
	if m.mcp == nil || m.mcp.review != nil {
		t.Fatalf("Esc did not leave the review: %+v", m.mcp)
	}
}

func TestMCPReviewIgnoresInstallKeysUntilItHasBeenOnScreenAMoment(t *testing.T) {
	m, k := mcpAddRig(t, nil)
	mcpReviewDelay = 400 * time.Millisecond
	startAdd(m, "npx -y @acme/mcp")
	run(m, keyRune(m, 'y'))
	if strings.Contains(k.seen(), "/mcp/install") || m.mcp.review == nil {
		t.Fatalf("a key pressed as the review appeared installed it:\n%s", k.seen())
	}
	m.mcp.review.armedAt = time.Now().Add(-time.Second)
	run(m, keyRune(m, 'y'))
	if !strings.Contains(k.seen(), "/mcp/install") {
		t.Fatalf("the review never armed:\n%s", k.seen())
	}
}

func TestMCPInputLineSaysItIsShownAsTyped(t *testing.T) {
	m, _ := mcpAddRig(t, nil)
	run(m, keyRune(m, 'n'))
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "shown as typed") {
		t.Fatalf("no note about the echoed line:\n%s", v)
	}
}
