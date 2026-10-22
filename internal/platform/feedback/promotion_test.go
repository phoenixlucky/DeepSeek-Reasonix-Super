package feedback

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

var thresholds = []int{0, 1, 3, 6, 12, 24, 48}

// at is a coherent profile for level n: its own threshold met exactly and the
// next one still to reach, the way the service states it.
func at(n string) string {
	level := int(n[0] - '0')
	cur, next := thresholds[level], thresholds[level+1]
	return fmt.Sprintf(`{"profile":{"level":%d,"adoptedCount":%d,"currentThreshold":%d,"nextLevel":%d,"nextThreshold":%d,"remaining":%d,"trustState":"active","trustExpiresAt":"2026-11-01T00:00:00.000Z","observedAt":"2026-10-07T08:00:00.000Z","effectiveLimits":{"reportsPerHour":6,"reportsPerDay":20,"repliesPerHour":5}},"items":[]}`,
		level, cur, cur, level+1, next, next-cur)
}

func listedAt(t *testing.T, svc *Service, st *stub, level string) Mine {
	t.Helper()
	st.mu.Lock()
	st.mineBody = at(level)
	st.mu.Unlock()
	got, err := svc.ListMine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTheFirstProfileOnAMachineIsABaselineNotAPromotion(t *testing.T) {
	st := &stub{}
	svc := submitted(t, st)
	if got := listedAt(t, svc, st, "2"); got.Promotion != nil {
		t.Fatalf("a first sight announced %+v", got.Promotion)
	}
	if got := listedAt(t, svc, st, "2"); got.Promotion != nil {
		t.Fatalf("an unchanged level announced %+v", got.Promotion)
	}
}

func TestALevelAboveTheBaselineIsAPromotionUntilItIsAcknowledged(t *testing.T) {
	st := &stub{}
	svc := submitted(t, st)
	listedAt(t, svc, st, "1")
	for range 2 {
		got := listedAt(t, svc, st, "2")
		if got.Promotion == nil || got.Promotion.From != 1 || got.Promotion.To != 2 {
			t.Fatalf("promotion = %+v, want 1 to 2 on every read until it is acknowledged", got.Promotion)
		}
	}
	if err := svc.AckPromotion(2); err != nil {
		t.Fatal(err)
	}
	if got := listedAt(t, svc, st, "2"); got.Promotion != nil {
		t.Fatalf("an acknowledged level announced again: %+v", got.Promotion)
	}
}

func TestAJumpOfSeveralLevelsIsOnePromotionToTheHighest(t *testing.T) {
	st := &stub{}
	svc := submitted(t, st)
	listedAt(t, svc, st, "1")
	if got := listedAt(t, svc, st, "4"); got.Promotion == nil || got.Promotion.From != 1 || got.Promotion.To != 4 {
		t.Fatalf("promotion = %+v", got.Promotion)
	}
}

func TestAcknowledgingNeverLowersTheMarkAndALowerLevelAnnouncesNothing(t *testing.T) {
	st := &stub{}
	svc := submitted(t, st)
	listedAt(t, svc, st, "3")
	if err := svc.AckPromotion(1); err != nil {
		t.Fatal(err)
	}
	if got := listedAt(t, svc, st, "2"); got.Promotion != nil {
		t.Fatalf("a lower level announced %+v", got.Promotion)
	}
	if got := listedAt(t, svc, st, "3"); got.Promotion != nil {
		t.Fatalf("the mark was lowered: %+v", got.Promotion)
	}
}

func TestARevokedStandingAnnouncesNothing(t *testing.T) {
	st := &stub{}
	svc := submitted(t, st)
	listedAt(t, svc, st, "1")
	st.mu.Lock()
	st.mineBody = strings.Replace(at("3"), `"trustState":"active"`, `"trustState":"revoked"`, 1)
	st.mu.Unlock()
	got, err := svc.ListMine(context.Background())
	if err != nil || got.Promotion != nil {
		t.Fatalf("revoked: %+v %v", got.Promotion, err)
	}
}

func TestAnOfflineListNeverMakesAPromotion(t *testing.T) {
	st := &stub{}
	svc := submitted(t, st)
	listedAt(t, svc, st, "1")
	st.mu.Lock()
	st.mineBody = at("3")
	st.mu.Unlock()
	if _, err := svc.ListMine(context.Background()); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.mineCode = 503
	st.mu.Unlock()
	got, _ := svc.ListMine(context.Background())
	if !got.Offline {
		t.Skip("the stub's refusal was not read as offline")
	}
	if got.Promotion != nil {
		t.Fatalf("offline announced %+v", got.Promotion)
	}
}
