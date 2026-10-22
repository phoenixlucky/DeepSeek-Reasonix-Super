package delegation

import (
	"time"

	"reasonix/internal/base/tokencount"
)

// subagentRateWindow is how much of a child's recent stream a reported rate
// reads from; subagentRateReportEvery paces how often a streaming child's
// status carries a fresh one.
const (
	subagentRateWindow      = 3 * time.Second
	subagentRateReportEvery = time.Second
)

type rateSample struct {
	at     time.Time
	tokens int
}

// rateMeter turns streamed token estimates into tokens per second over a
// sliding window. The first sample is only an anchor for where the stream
// started, so a steady stream reads as its true rate from the first second on.
type rateMeter struct{ samples []rateSample }

func (m *rateMeter) add(at time.Time, tokens int) {
	m.samples = append(m.samples, rateSample{at: at, tokens: tokens})
	m.trim(at)
}

// trim keeps one sample at or before the window's edge as the anchor, and drops
// an anchor the stream had already gone quiet after: that is a restart.
func (m *rateMeter) trim(now time.Time) {
	for len(m.samples) >= 2 && (!m.samples[1].at.After(now.Add(-subagentRateWindow)) || m.samples[1].at.Sub(m.samples[0].at) > subagentRateWindow) {
		m.samples = m.samples[1:]
	}
}

// rate reports the estimate, or false while less than a second of stream is
// known: a rate over a few milliseconds would be noise.
func (m *rateMeter) rate(now time.Time) (int, bool) {
	m.trim(now)
	if len(m.samples) < 2 {
		return 0, false
	}
	span := now.Sub(m.samples[0].at)
	if span < time.Second {
		return 0, false
	}
	sum := 0
	for _, s := range m.samples[1:] {
		sum += s.tokens
	}
	return int(float64(sum)/span.Seconds() + 0.5), true
}

func (m *rateMeter) reset() { m.samples = nil }

// rateNowLocked is the estimate the status event of a streaming phase carries;
// any other phase reports none, because a rate of zero in a tool call means
// nothing is wrong.
func (t *subagentProgressTracker) rateNowLocked() int {
	if t.phase != subagentPhaseReasoning && t.phase != subagentPhaseResponding {
		return 0
	}
	rate, _ := t.meter.rate(t.merger.clock.Now())
	return rate
}

// streamedLocked folds a streamed delta into the meter and, once a second,
// queues a status event with the fresh estimate.
func (t *subagentProgressTracker) streamedLocked(text string) {
	now := t.merger.clock.Now()
	if len(t.meter.samples) == 0 {
		t.rateSent = now
	}
	t.meter.add(now, tokencount.Text(text))
	if now.Sub(t.rateSent) < subagentRateReportEvery {
		return
	}
	if rate, ok := t.meter.rate(now); ok {
		t.rateSent = now
		t.merger.statusEventRate(t.childID, t.phase, rate)
	}
}
