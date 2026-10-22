package update

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoProbe struct {
	reads atomic.Int32
	err   string
	gate  chan struct{}
}

func (p *memoProbe) read(context.Context) VersionHub {
	p.reads.Add(1)
	if p.gate != nil {
		<-p.gate
	}
	return VersionHub{Current: "1.0.0", Err: p.err}
}

func TestHubReadsTheCatalogOnceForSeveralAskers(t *testing.T) {
	var m hubMemo
	p := &memoProbe{}
	now := time.Unix(1000, 0)
	for range 3 {
		m.get(context.Background(), "k", false, func() time.Time { return now }, p.read)
	}
	if got := p.reads.Load(); got != 1 {
		t.Fatalf("catalog read %d times for three askers inside the fresh window", got)
	}
	now = now.Add(hubFresh + time.Second)
	m.get(context.Background(), "k", false, func() time.Time { return now }, p.read)
	if got := p.reads.Load(); got != 2 {
		t.Fatalf("a stale answer was served: %d reads", got)
	}
}

func TestHubRemembersAFailureBrieflyAndRefreshOvercomesIt(t *testing.T) {
	var m hubMemo
	p := &memoProbe{err: "mirror unreachable"}
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	m.get(context.Background(), "k", false, clock, p.read)
	m.get(context.Background(), "k", false, clock, p.read)
	if got := p.reads.Load(); got != 1 {
		t.Fatalf("a failed read was repeated at once: %d reads", got)
	}
	m.get(context.Background(), "k", true, clock, p.read)
	if got := p.reads.Load(); got != 2 {
		t.Fatalf("refresh did not read again: %d reads", got)
	}
	now = now.Add(hubFailedFresh + time.Second)
	m.get(context.Background(), "k", false, clock, p.read)
	if got := p.reads.Load(); got != 3 {
		t.Fatalf("a failure outlived its window: %d reads", got)
	}
}

func TestHubRereadsWhenTheInstallOrPinChanges(t *testing.T) {
	var m hubMemo
	p := &memoProbe{}
	clock := func() time.Time { return time.Unix(1000, 0) }
	m.get(context.Background(), "1.0.0|", false, clock, p.read)
	m.get(context.Background(), "1.0.0|2.0.0", false, clock, p.read)
	if got := p.reads.Load(); got != 2 {
		t.Fatalf("a pin change was answered from the old read: %d reads", got)
	}
}

func TestHubJoinsAReadAlreadyUnderWay(t *testing.T) {
	var m hubMemo
	p := &memoProbe{gate: make(chan struct{})}
	clock := func() time.Time { return time.Unix(1000, 0) }
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { m.get(context.Background(), "k", false, clock, p.read) })
	}
	for p.reads.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(p.gate)
	wg.Wait()
	if got := p.reads.Load(); got != 1 {
		t.Fatalf("four concurrent askers started %d reads", got)
	}
}

func TestHubDoesNotKeepWhatAGoneCallerSaw(t *testing.T) {
	var m hubMemo
	p := &memoProbe{err: "context canceled"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	clock := func() time.Time { return time.Unix(1000, 0) }
	m.get(ctx, "k", false, clock, p.read)
	m.get(context.Background(), "k", false, clock, p.read)
	if got := p.reads.Load(); got != 2 {
		t.Fatalf("an abandoned read answered the next caller: %d reads", got)
	}
}
