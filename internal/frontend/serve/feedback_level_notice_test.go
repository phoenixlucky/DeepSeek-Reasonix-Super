package serve

import (
	"encoding/json"
	"net/http"
	"testing"

	"reasonix/internal/base/testenv"
)

func minePromoted(level int) string {
	th := []int{0, 1, 3, 6, 12, 24, 48}
	cur, next := th[level], th[level+1]
	return `{"profile":{"level":` + itoa(level) + `,"adoptedCount":` + itoa(cur) + `,"currentThreshold":` + itoa(cur) + `,"nextLevel":` + itoa(level+1) + `,"nextThreshold":` + itoa(next) + `,"remaining":` + itoa(next-cur) + `,"trustState":"active","trustExpiresAt":"2026-11-01T00:00:00.000Z","observedAt":"2026-10-07T08:00:00.000Z","effectiveLimits":{"reportsPerHour":6,"reportsPerDay":20,"repliesPerHour":5}},"items":[]}`
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestTheLevelNoticeSwitchRoundTripsAndStartsOff(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	srv := feedbackServer(t, &feedbackStub{}, true)
	var got struct {
		Enabled bool `json:"enabled"`
	}
	read := func() bool {
		resp, err := http.Get(srv.URL + "/feedback-level-notices")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		got.Enabled = false
		_ = json.NewDecoder(resp.Body).Decode(&got)
		return got.Enabled
	}
	if read() {
		t.Fatal("level notices start on")
	}
	if resp, out := feedbackPost(t, srv.URL+"/feedback-level-notices", map[string]bool{"enabled": true}); resp.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", resp.StatusCode, out)
	}
	if !read() {
		t.Fatal("the switch did not hold")
	}
	if resp, _ := feedbackPost(t, srv.URL+"/feedback-level-notices", map[string]any{}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a body without the switch = %d, want 400", resp.StatusCode)
	}
}

func TestAPromotionReachesTheListOnlyWithTheSwitchOnAndStaysUntilAcknowledged(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	stub := &feedbackStub{}
	srv := feedbackServer(t, stub, true)
	feedbackPost(t, srv.URL+"/feedback", map[string]any{"category": "bug", "body": "x", "displayName": "kim"})
	feedbackPost(t, srv.URL+"/feedback-level-notices", map[string]bool{"enabled": true})
	setLevel := func(n int) {
		stub.mu.Lock()
		stub.mine = minePromoted(n)
		stub.mu.Unlock()
	}
	setLevel(1)
	feedbackGetMine(t, srv.URL)
	setLevel(2)
	for range 2 {
		got := feedbackGetMine(t, srv.URL)
		if got.Promotion == nil || got.Promotion.From != 1 || got.Promotion.To != 2 {
			t.Fatalf("promotion = %+v", got.Promotion)
		}
	}
	if resp, out := feedbackPost(t, srv.URL+"/feedback/promotion/ack", map[string]int{"level": 2}); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ack: %d %s", resp.StatusCode, out)
	}
	if got := feedbackGetMine(t, srv.URL); got.Promotion != nil {
		t.Fatalf("an acknowledged promotion came back: %+v", got.Promotion)
	}
	if resp, _ := feedbackPost(t, srv.URL+"/feedback/promotion/ack", map[string]string{"level": "high"}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a level that is not a number = %d, want 400", resp.StatusCode)
	}
}
