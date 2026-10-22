package jobs

import (
	"context"
	"io"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
)

func TestPresentationRidesTheViewAndIsBounded(t *testing.T) {
	m := NewManager(event.Discard)
	defer m.Close()
	long := strings.Repeat("a", MaxDetailRunes+500)
	j := m.StartForSession("", "bash", "short", func(ctx context.Context, _ io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}, WithPresentation(" Build the site ", long))
	views := m.Running()
	if len(views) != 1 || views[0].ID != j.ID {
		t.Fatalf("running = %+v", views)
	}
	if views[0].Description != "Build the site" {
		t.Errorf("description = %q", views[0].Description)
	}
	if got := []rune(views[0].Detail); len(got) != MaxDetailRunes+1 || got[len(got)-1] != '…' {
		t.Errorf("detail is %d runes, want %d plus an ellipsis", len(got), MaxDetailRunes)
	}
}

func TestALongDescriptionIsCutForDisplay(t *testing.T) {
	m := NewManager(event.Discard)
	defer m.Close()
	m.StartForSession("", "bash", "short", func(ctx context.Context, _ io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}, WithPresentation(strings.Repeat("名", MaxDescriptionRunes+50), "cmd"))
	got := []rune(m.Running()[0].Description)
	if len(got) != MaxDescriptionRunes+1 || got[len(got)-1] != '…' {
		t.Errorf("description is %d runes, want %d plus an ellipsis", len(got), MaxDescriptionRunes)
	}
}

func TestAJobWithoutPresentationIsUnchanged(t *testing.T) {
	m := NewManager(event.Discard)
	defer m.Close()
	m.Start("bash", "plain", func(ctx context.Context, _ io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	v := m.Running()[0]
	if v.Label != "plain" || v.Description != "" || v.Detail != "" {
		t.Fatalf("view = %+v", v)
	}
}
