package control

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/platform/feedback"
)

var noticeThresholds = []int{0, 1, 3, 6, 12, 24, 48}

func mineAtLevel(level int) string {
	cur, next := noticeThresholds[level], noticeThresholds[level+1]
	return fmt.Sprintf(`{"profile":{"level":%d,"adoptedCount":%d,"currentThreshold":%d,"nextLevel":%d,"nextThreshold":%d,"remaining":%d,"trustState":"active","trustExpiresAt":"2026-11-01T00:00:00.000Z","observedAt":"2026-10-07T08:00:00.000Z","effectiveLimits":{"reportsPerHour":6,"reportsPerDay":20,"repliesPerHour":5}},"items":[]}`,
		level, cur, cur, level+1, next, next-cur)
}

func noticeRig(t *testing.T, on bool) *feedbackRig {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	r := sentRig(t)
	if err := r.c.SaveFeedbackLevelNotices(on); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *feedbackRig) listAt(t *testing.T, level int) feedback.Mine {
	t.Helper()
	r.edit(func(r *feedbackRig) { r.mine = mineAtLevel(level) })
	got, err := r.c.ListFeedback(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTheLevelNoticeSettingIsOffByDefaultAndPersists(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	r := newFeedbackRig(t, feedback.SurfaceTUI, true)
	if r.c.FeedbackLevelNotices().Enabled {
		t.Fatal("level notices are on by default")
	}
	if err := r.c.SaveFeedbackLevelNotices(true); err != nil {
		t.Fatal(err)
	}
	if !r.c.FeedbackLevelNotices().Enabled {
		t.Fatal("the switch did not hold")
	}
}

func TestWithTheSwitchOffAPromotionIsNeverSurfacedAndTheBaselineStillFollows(t *testing.T) {
	r := noticeRig(t, false)
	r.listAt(t, 1)
	if got := r.listAt(t, 2); got.Promotion != nil {
		t.Fatalf("the switch is off but %+v came through", got.Promotion)
	}
	if err := r.c.SaveFeedbackLevelNotices(true); err != nil {
		t.Fatal(err)
	}
	if got := r.listAt(t, 2); got.Promotion != nil {
		t.Fatalf("turning the switch on announced an old promotion: %+v", got.Promotion)
	}
	if got := r.listAt(t, 3); got.Promotion == nil || got.Promotion.From != 2 || got.Promotion.To != 3 {
		t.Fatalf("a new promotion after turning it on = %+v", got.Promotion)
	}
}

func TestWithTheSwitchOnAPromotionStaysUntilTheControllerIsToldItWasSeen(t *testing.T) {
	r := noticeRig(t, true)
	r.listAt(t, 1)
	for range 2 {
		if got := r.listAt(t, 2); got.Promotion == nil || got.Promotion.To != 2 {
			t.Fatalf("promotion = %+v", got.Promotion)
		}
	}
	if err := r.c.AckFeedbackPromotion(2); err != nil {
		t.Fatal(err)
	}
	if got := r.listAt(t, 2); got.Promotion != nil {
		t.Fatalf("an acknowledged promotion came back: %+v", got.Promotion)
	}
}

func TestTheTerminalListSaysALevelUpOnceAndOnlyWithTheSwitchOn(t *testing.T) {
	for _, on := range []bool{true, false} {
		r := noticeRig(t, on)
		r.listAt(t, 1)
		r.edit(func(r *feedbackRig) { r.mine = mineAtLevel(2) })
		r.c.Submit("/feedback list")
		if on {
			got := r.last(t, "up from")
			if !strings.Contains(got, "2") {
				t.Fatalf("line = %q", got)
			}
		} else {
			r.last(t, "Level 2")
		}
		r.mu.Lock()
		r.texts = nil
		r.mu.Unlock()
		r.c.Submit("/feedback list")
		got := r.last(t, "Level 2")
		if strings.Contains(got, "up from") {
			t.Fatalf("on=%v: the level-up line was said twice: %q", on, got)
		}
	}
}
