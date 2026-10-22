package control

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

func gatedScript(extra ...[]provider.Chunk) *scriptedTurns {
	turns := [][]provider.Chunk{
		{toolCallChunk("t0", "todo_write", `{"todos":[
			{"step_id":"n1","content":"item one","status":"in_progress"},
			{"step_id":"n2","content":"item two","status":"pending"}]}`), {Type: provider.ChunkDone}},
		{toolCallChunk("w1", "write_file", `{"path":"notes/item-one.md"}`), {Type: provider.ChunkDone}},
		{toolCallChunk("g1", "await_user", `{"step_id":"n1","need":"your view on item one"}`), {Type: provider.ChunkDone}},
		textTurn("Here is item one. What do you make of it?"),
	}
	return &scriptedTurns{turns: append(turns, extra...)}
}

func withAwaitDelay(t *testing.T, d time.Duration) {
	t.Helper()
	awaitDelayOverride = func() (time.Duration, bool) { return d, true }
	t.Cleanup(func() { awaitDelayOverride = nil })
}

// With the user's ask-timeout on, an unanswered hand-back is continued by the
// host, told nobody answered, and the continuation is a host turn.
func TestUnansweredHandBackContinuesAfterTheTimeout(t *testing.T) {
	withAwaitDelay(t, 40*time.Millisecond)
	prov := gatedScript(textTurn("Assumptions: item two goes first. Done."))
	c, done, notices := readinessGatedController(t, prov)

	c.Submit("walk the list with me")
	<-done
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("the list was not continued; provider calls = %d", prov.call)
	}
	if prov.call != 5 {
		t.Fatalf("provider calls = %d, want the one continuation", prov.call)
	}
	var timedOut bool
	for _, n := range notices() {
		if n.Code == event.NoticeCodeAwaitTimedOut {
			timedOut = n.Detail == "your view on item one"
		}
	}
	if !timedOut {
		t.Fatalf("no await_timed_out notice carrying the need among %+v", notices())
	}
}

// Off, the default, waits as long as it takes.
func TestHandBackWaitsWhenTheTimeoutIsOff(t *testing.T) {
	withAwaitDelay(t, 0)
	prov := gatedScript(textTurn("must not run"))
	c, done, _ := readinessGatedController(t, prov)

	c.Submit("walk the list with me")
	<-done
	time.Sleep(150 * time.Millisecond)
	if prov.call != 4 {
		t.Fatalf("provider calls = %d, want the list to keep waiting", prov.call)
	}
}

// The user answering first cancels the wait; closing the session does too.
func TestAnsweringOrClosingCancelsTheWait(t *testing.T) {
	withAwaitDelay(t, time.Hour)
	prov := gatedScript(textTurn("ok"))
	c, done, _ := readinessGatedController(t, prov)
	c.Submit("walk the list with me")
	<-done
	for range 100 {
		c.awaitResume.mu.Lock()
		armed := c.awaitResume.timer != nil
		c.awaitResume.mu.Unlock()
		if armed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.Submit("my view: fine")
	<-done
	c.awaitResume.mu.Lock()
	defer c.awaitResume.mu.Unlock()
	if c.awaitResume.timer != nil {
		t.Fatal("a turn that read the user's answer left the wait armed")
	}
}

func TestTimedOutPromptAsksForAssumptions(t *testing.T) {
	p := awaitTimedOutPrompt("pick")
	if !strings.Contains(p, "Assumptions:") || !strings.Contains(p, "pick") {
		t.Fatalf("prompt = %q", p)
	}
}

func handBack(id string) []provider.Chunk {
	return []provider.Chunk{toolCallChunk(id, "await_user", `{"step_id":"n1","need":"your view on item one"}`), {Type: provider.ChunkDone}}
}

// A model that hands the list back every time is continued exactly the limit,
// then left waiting; the user speaking again clears the count.
func TestAutoContinuationStopsAtTheLimitAndAUserMessageResetsIt(t *testing.T) {
	withAwaitDelay(t, 30*time.Millisecond)
	var script [][]provider.Chunk
	for i := range awaitResumeLimit {
		script = append(script, handBack(fmt.Sprintf("g%d", i+2)), textTurn("Assumptions: none. Still need you."))
	}
	script = append(script, textTurn("must not run"))
	prov := gatedScript(script...)
	c, done, notices := readinessGatedController(t, prov)

	c.Submit("walk the list with me")
	for range 1 + awaitResumeLimit {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("turn did not finish; provider calls = %d", prov.call)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range notices() {
			if n.Code == event.NoticeCodeAwaitGaveUp {
				goto gaveUp
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no await_gave_up notice among %+v", notices())
gaveUp:
	want := 4 + 2*awaitResumeLimit
	if prov.call != want {
		t.Fatalf("provider calls = %d, want %d: the host continued %d times", prov.call, want, awaitResumeLimit)
	}
	c.awaitResume.mu.Lock()
	runs := c.awaitResume.runs
	c.awaitResume.mu.Unlock()
	if runs != awaitResumeLimit {
		t.Fatalf("runs = %d before the user spoke, want %d", runs, awaitResumeLimit)
	}
	c.Submit("my view: fine")
	<-done
	c.awaitResume.mu.Lock()
	defer c.awaitResume.mu.Unlock()
	if c.awaitResume.runs != 0 {
		t.Fatalf("runs = %d after the user spoke, want 0", c.awaitResume.runs)
	}
}
