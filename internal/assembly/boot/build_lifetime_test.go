package boot

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBuildCancellationBeforePublication(t *testing.T) {
	_, sink := buildNoticeFixture(t, "")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctrl, err := Build(ctx, Options{Sink: sink, OnPhase: func(phase Phase) {
		if phase.Name == "config" {
			cancel()
		}
	}})
	if ctrl != nil {
		ctrl.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled construction returned controller=%v err=%v", ctrl != nil, err)
	}
}

func TestConstructionLifetimeDetachesOnlyAfterSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	life, detach, closeLifetime := constructionLifetime(ctx)
	defer closeLifetime()
	if err := detach(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-life.Done():
		t.Fatal("published lifetime died with its request")
	default:
	}
	closeLifetime()
	select {
	case <-life.Done():
	case <-time.After(time.Second):
		t.Fatal("runtime close did not retire its lifetime")
	}
}
