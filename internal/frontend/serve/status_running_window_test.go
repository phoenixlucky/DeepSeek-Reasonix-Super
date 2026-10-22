package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/session/control"
)

// The window a client reads while the kernel is still delivering TurnDone: the
// turn is over, so `running` is false, and `busy` stays true because a new turn
// is still refused. A window that polled in that gap and heard "running" kept a
// finished turn's tab breathing.
func TestStatusSaysTheTurnEndedWhileAdmissionStaysBusyDuringItsDelivery(t *testing.T) {
	bc := NewBroadcaster()
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var first atomic.Int32
	sink := event.FuncSink(func(e event.Event) {
		bc.Emit(e)
		if e.Kind == event.TurnDone && first.Add(1) == 1 {
			entered <- struct{}{}
			<-release
		}
	})
	ctrl := control.New(control.Options{Runner: fakeRunner{got: make(chan string, 1)}, Sink: sink})
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	defer srv.Close()
	submitted, err := http.Post(srv.URL+"/submit", "application/json", strings.NewReader(`{"input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	submitted.Body.Close()
	if submitted.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /submit = %d, want 202", submitted.StatusCode)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never reached its TurnDone")
	}

	var st struct {
		Running bool `json:"running"`
		Busy    bool `json:"busy"`
	}
	resp, err := http.Get(srv.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Running || !st.Busy {
		t.Fatalf("status during the delivery window = running %v busy %v, want running false and busy true", st.Running, st.Busy)
	}
	close(release)
}
