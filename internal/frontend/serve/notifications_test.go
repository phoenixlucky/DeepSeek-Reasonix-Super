package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"reasonix/internal/base/i18n"
	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/platform/notify"
)

type sentLog struct {
	mu   sync.Mutex
	sent []notify.Message
}

func (s *sentLog) Send(m notify.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}

func (s *sentLog) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

func notifyServer(t *testing.T, sender notify.Sender) (*httptest.Server, *Hub) {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	h := NewHub(HubOptions{Notifications: notify.NewSettings(config.NotificationsConfig{}), NotifySender: sender})
	srv := httptest.NewServer(operatorHandler(h))
	t.Cleanup(srv.Close)
	return srv, h
}

func announce(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	resp, err := http.Post(srv.URL+"/notifications/feedback-reply", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestFeedbackReplyNotificationFollowsBothSwitches(t *testing.T) {
	log := &sentLog{}
	srv, h := notifyServer(t, log)
	for _, c := range []struct {
		name string
		cfg  NotifyPrefs
		want int
	}{
		{"master off", NotifyPrefs{FeedbackReply: true}, 0},
		{"kind off", NotifyPrefs{Enabled: true}, 0},
		{"both on", NotifyPrefs{Enabled: true, FeedbackReply: true}, 1},
	} {
		before := log.count()
		if _, err := h.SetNotifyPrefs(c.cfg); err != nil {
			t.Fatal(err)
		}
		if got := announce(t, srv); got != http.StatusNoContent {
			t.Fatalf("%s: status %d", c.name, got)
		}
		if got := log.count() - before; got != c.want {
			t.Errorf("%s: sent %d, want %d", c.name, got, c.want)
		}
	}
	want := i18n.CatalogFor(config.LoadForEdit(config.UserConfigPath()).DesktopLanguage()).NotifyFeedbackReply
	if body := log.sent[0].Body; body == "" || body != want {
		t.Errorf("body %q is not the neutral catalogue text", body)
	}
}

func TestFeedbackReplyPrefsRoundTripAndDefaultOn(t *testing.T) {
	srv, h := notifyServer(t, &sentLog{})
	if !h.NotifyPrefs().FeedbackReply {
		t.Error("an install that never wrote the key should default to on")
	}
	got, err := h.SetNotifyPrefs(NotifyPrefs{Enabled: true})
	if err != nil || got.FeedbackReply {
		t.Fatalf("set off: %+v %v", got, err)
	}
	resp, err := http.Get(srv.URL + "/notifications")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var read NotifyPrefs
	if err := json.NewDecoder(resp.Body).Decode(&read); err != nil || read.FeedbackReply || !read.Enabled {
		t.Errorf("read %+v %v", read, err)
	}
}

func TestFeedbackReplyRouteAbsentWithoutASender(t *testing.T) {
	srv, _ := notifyServer(t, nil)
	if got := announce(t, srv); got == http.StatusNoContent {
		t.Errorf("a hub with no sender answered %d", got)
	}
}

func TestSoundIsOffByDefaultAndRoundTripsIndependentlyOfTheToast(t *testing.T) {
	_, h := notifyServer(t, &sentLog{})
	if h.NotifyPrefs().Sound {
		t.Fatal("an install that never wrote the key must not make a sound")
	}
	got, err := h.SetNotifyPrefs(NotifyPrefs{Sound: true, TurnDone: true})
	if err != nil || !got.Sound || got.Enabled {
		t.Fatalf("set sound only: %+v %v", got, err)
	}
	if again := h.NotifyPrefs(); !again.Sound || again.Enabled {
		t.Fatalf("durable read: %+v", again)
	}
}

// Saving the switches rewrites the whole notifications table, so a switch the
// prefs do not carry would be reset to off by the first save from any window.
func TestDiskLowPrefsRoundTripAndDefaultOn(t *testing.T) {
	srv, h := notifyServer(t, &sentLog{})
	if !h.NotifyPrefs().DiskLow {
		t.Error("an install that never wrote the key should default to on")
	}
	got, err := h.SetNotifyPrefs(NotifyPrefs{Enabled: true, TurnDone: true, DiskLow: true})
	if err != nil || !got.DiskLow {
		t.Fatalf("keep on across a save that changed something else: %+v %v", got, err)
	}
	got, err = h.SetNotifyPrefs(NotifyPrefs{Enabled: true})
	if err != nil || got.DiskLow {
		t.Fatalf("set off: %+v %v", got, err)
	}
	resp, err := http.Get(srv.URL + "/notifications")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var read NotifyPrefs
	if err := json.NewDecoder(resp.Body).Decode(&read); err != nil || read.DiskLow || !read.Enabled {
		t.Errorf("read %+v %v", read, err)
	}
}

func putPrefs(t *testing.T, srv *httptest.Server, body string) NotifyPrefs {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/notifications", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got NotifyPrefs
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT = %d, %v", resp.StatusCode, err)
	}
	return got
}

func TestPhoneNotificationsAreOffUntilEachSwitchIsTurnedOnAndSurviveAnOlderPage(t *testing.T) {
	srv, h := notifyServer(t, &sentLog{})
	if got := h.NotifyPrefs().Phone; got != (PhoneNotifyPrefs{}) {
		t.Fatalf("phone switches on by default: %+v", got)
	}
	got := putPrefs(t, srv, `{"enabled":false,"phone":{"enabled":true,"approval":true,"input":false,"error":true}}`)
	if want := (PhoneNotifyPrefs{Enabled: true, Approval: true, Error: true}); got.Phone != want {
		t.Fatalf("phone = %+v, want %+v", got.Phone, want)
	}
	cfg := config.LoadForEdit(config.UserConfigPath()).Notifications.Phone
	if !cfg.Enabled || !cfg.Approval || cfg.Input || !cfg.Error {
		t.Fatalf("the durable setting = %+v", cfg)
	}
	// A page from before the switches existed sends none of them: they stay.
	got = putPrefs(t, srv, `{"enabled":true,"turnDone":true}`)
	if want := (PhoneNotifyPrefs{Enabled: true, Approval: true, Error: true}); got.Phone != want || !got.Enabled {
		t.Fatalf("after an older page wrote: phone = %+v enabled = %v", got.Phone, got.Enabled)
	}
	// Turning the master switch off is a write like any other.
	got = putPrefs(t, srv, `{"phone":{"enabled":false,"approval":true,"input":false,"error":true}}`)
	if got.Phone.Enabled {
		t.Fatalf("the master switch did not turn off: %+v", got.Phone)
	}
	if live := h.opts.Notifications.Load().Phone; live != (config.PhoneNotificationsConfig{Approval: true, Error: true}) {
		t.Fatalf("the running sinks were given %+v, want the setting just written", live)
	}
}
