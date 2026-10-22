package jobs

import (
	"strings"
	"testing"
	"time"

	"reasonix/internal/contract/event"
)

func TestStalledJobEmitsCodedNoticeWithTheQuietSpan(t *testing.T) {
	sink := &recordingSink{}
	m := NewManager(sink)
	defer m.Close()
	m.stalledWarning = 15 * time.Minute
	m.recordStalled("", "bash-16", "bash", "make build")
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for _, ev := range sink.events {
		if ev.Code != event.NoticeCodeJobStalled {
			continue
		}
		p, ok := event.DecodeJobNotice(ev.Detail)
		if !ok || p.Kind != "bash" || p.ID != "bash-16" || p.Label != "make build" || p.QuietSeconds != 900 || ev.Level != event.LevelWarn {
			t.Fatalf("stalled notice = %+v payload %+v", ev, p)
		}
		if !strings.Contains(ev.Text, "has written nothing to its own output for 15m0s") || !event.DetailIsPayload(ev.Code) {
			t.Fatalf("English fallback = %q, payload flag %v", ev.Text, event.DetailIsPayload(ev.Code))
		}
		return
	}
	t.Fatalf("no %s notice in %+v", event.NoticeCodeJobStalled, sink.events)
}
