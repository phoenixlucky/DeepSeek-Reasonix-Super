package remotecloud

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"reasonix/internal/platform/account"
)

func part(id string, index, total int, body []byte, method, path string) controllerCommand {
	c := controllerCommand{Version: 1, Type: "desktop.request.part", ID: id, Index: index, Total: total, Body: base64.RawURLEncoding.EncodeToString(body)}
	if index == 0 {
		c.Method, c.Path = method, path
	}
	return c
}

func TestUploadReassemblesInOrderAndHandsOverOnlyTheWhole(t *testing.T) {
	var u uploads
	now := time.Unix(1000, 0)
	chunks := [][]byte{bytes.Repeat([]byte("a"), uploadPartMax), bytes.Repeat([]byte("b"), uploadPartMax), []byte("tail")}
	for i, c := range chunks {
		req, done, err := u.add(part("u1", i, 3, c, http.MethodPost, "/attachments"), now)
		if err != nil {
			t.Fatalf("part %d: %v", i, err)
		}
		if done != (i == 2) {
			t.Fatalf("part %d: done = %v", i, done)
		}
		if done {
			if req.Method != http.MethodPost || req.Path != "/attachments" || !bytes.Equal(req.Body, bytes.Join(chunks, nil)) {
				t.Fatalf("reassembled = %q %q %d bytes", req.Method, req.Path, len(req.Body))
			}
		}
	}
	if u.current != nil {
		t.Fatal("a finished upload is still held")
	}
}

func TestUploadRefusesWhatCannotBeAPartAndDropsTheUpload(t *testing.T) {
	now := time.Unix(1000, 0)
	start := func() *uploads {
		u := &uploads{}
		if _, _, err := u.add(part("u", 0, 3, []byte("x"), http.MethodPost, "/p"), now); err != nil {
			t.Fatal(err)
		}
		return u
	}
	for name, tc := range map[string]struct {
		c    controllerCommand
		want error
	}{
		"out of order":       {part("u", 2, 3, []byte("x"), "", ""), errUploadOrder},
		"repeated index":     {part("u", 0, 3, []byte("y"), http.MethodPost, "/p"), nil}, // a restart of the same id is allowed
		"another id mid-way": {part("other", 1, 3, []byte("x"), "", ""), errUploadOrder},
		"total changed":      {part("u", 1, 4, []byte("x"), "", ""), errUploadOrder},
		"empty part":         {part("u", 1, 3, nil, "", ""), errUploadInvalid},
		"oversize part":      {part("u", 1, 3, make([]byte, uploadPartMax+1), "", ""), errUploadInvalid},
		"index past total":   {part("u", 3, 3, []byte("x"), "", ""), errUploadInvalid},
		"total over limit":   {part("u", 1, maxUploadBody/uploadPartMax+1, []byte("x"), "", ""), errUploadInvalid},
		"no method on first": {part("v", 0, 3, []byte("x"), "", ""), errUploadInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			u := start()
			_, _, err := u.add(tc.c, now)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want.Error()) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.c.ID == "u" && u.current != nil {
				t.Fatal("a refused part left its upload behind")
			}
		})
	}
}

func TestUploadCountsBytesAsTheyArriveAndStopsAtTheLimit(t *testing.T) {
	var u uploads
	now := time.Unix(1000, 0)
	total := maxUploadBody / uploadPartMax
	full := bytes.Repeat([]byte("z"), uploadPartMax)
	var err error
	for i := range total {
		if _, _, err = u.add(part("big", i, total, full, http.MethodPost, "/p"), now); err != nil {
			t.Fatalf("part %d: %v", i, err)
		}
	}
	if u.current != nil {
		t.Fatal("the upload at the limit did not finish")
	}
	// A page that declares few parts and keeps sending is held to what arrives, not what it said.
	if _, _, err = u.add(part("liar", 0, 2, full, http.MethodPost, "/p"), now); err != nil {
		t.Fatal(err)
	}
	u.current.body = make([]byte, maxUploadBody-1)
	if _, _, err = u.add(part("liar", 1, 2, full, "", ""), now); err == nil || !strings.Contains(err.Error(), errUploadTooBig.Error()) {
		t.Fatalf("err = %v, want too big", err)
	}
	if u.current != nil {
		t.Fatal("an upload past the limit is still held")
	}
}

func TestUploadExpiresWhenNextPartOrAnyCommandArrivesLate(t *testing.T) {
	var u uploads
	t0 := time.Unix(1000, 0)
	if _, _, err := u.add(part("slow", 0, 2, []byte("x"), http.MethodPost, "/p"), t0); err != nil {
		t.Fatal(err)
	}
	u.expire(t0.Add(uploadIdle - time.Second))
	if u.current == nil {
		t.Fatal("expired early")
	}
	u.expire(t0.Add(uploadIdle))
	if u.current != nil {
		t.Fatal("an idle upload outlived its time on the next command of any kind")
	}
	if _, _, err := u.add(part("slow2", 0, 2, []byte("x"), http.MethodPost, "/p"), t0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := u.add(part("slow2", 1, 2, []byte("y"), "", ""), t0.Add(uploadIdle)); err == nil || !strings.Contains(err.Error(), errUploadExpired.Error()) {
		t.Fatalf("err = %v, want expired", err)
	}
}

// channel is a host and one controller joined by a real websocket pair and the
// real sealing, for tests that go through Host.handle as the relay would.
type channel struct {
	host         *Host
	deviceWire   *websocket.Conn
	controller   *websocket.Conn
	cipher       *sessionCipher
	device       *ecdh.PrivateKey
	sessions     map[string]*controllerSession
	deviceID     string
	connectionID string
	ready        map[string]any
	sent         int64
	stop         func()
}

func newChannel(t *testing.T, desktop DesktopService) *channel {
	t.Helper()
	connections := make(chan *websocket.Conn, 1)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		connections <- conn
		<-done
	}))
	controllerWire, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	deviceWire := <-connections
	var once sync.Once
	stop := func() {
		once.Do(func() {
			controllerWire.Close()
			deviceWire.Close()
			close(done)
			server.Close()
		})
	}
	t.Cleanup(stop)
	device, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	ch := &channel{
		host: &Host{name: "Home Mac", version: "test", desktop: desktop, presence: &presenceStub{}}, deviceWire: deviceWire, controller: controllerWire,
		device: device, stop: stop, sessions: map[string]*controllerSession{}, deviceID: strings.Repeat("d", 64), connectionID: strings.Repeat("c", 32),
	}
	greeting, _ := json.Marshal(hello{Version: protocolVersion, Type: "hello", PublicKey: base64.RawURLEncoding.EncodeToString(controller.PublicKey().Bytes()), Salt: base64.RawURLEncoding.EncodeToString(salt)})
	if err := ch.host.handle(t.Context(), deviceWire, &identity{DeviceID: ch.deviceID}, device, ch.sessions, mustJSON(t, gatewayMessage{Type: "controller_message", ConnectionID: ch.connectionID, Payload: string(greeting)})); err != nil {
		t.Fatal(err)
	}
	back, _ := json.Marshal(hello{Version: protocolVersion, Type: "hello", PublicKey: base64.RawURLEncoding.EncodeToString(device.PublicKey().Bytes()), Salt: base64.RawURLEncoding.EncodeToString(salt)})
	ch.cipher, err = newSessionCipher(controller, ch.deviceID, string(back))
	if err != nil {
		t.Fatal(err)
	}
	ch.ready = readDirected(t, controllerWire, ch.cipher)
	return ch
}

// send seals a command as the page would, stamped with the session and the next
// sequence number, and hands it to the host.
func (ch *channel) send(t *testing.T, command controllerCommand, scopes ...account.RemoteCapability) {
	t.Helper()
	ch.sent++
	command.Session = ch.sessions[ch.connectionID].nonce
	command.Seq = ch.sent
	wire, err := ch.cipher.seal(command)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.host.handle(t.Context(), ch.deviceWire, &identity{DeviceID: ch.deviceID}, ch.device, ch.sessions, mustJSON(t, gatewayMessage{
		Type: "controller_message", ConnectionID: ch.connectionID, Scopes: scopes, Payload: wire,
	})); err != nil {
		t.Fatal(err)
	}
}

type recordingDesktop struct {
	mu       sync.Mutex
	requests []DesktopRequest
}

func (d *recordingDesktop) CloudDesktop(_ context.Context, r DesktopRequest, _ string) (DesktopResponse, error) {
	d.mu.Lock()
	d.requests = append(d.requests, r)
	d.mu.Unlock()
	return DesktopResponse{Status: http.StatusOK, ContentType: "application/json", Body: []byte(`{"ok":true}`)}, nil
}

func (d *recordingDesktop) seen() []DesktopRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DesktopRequest(nil), d.requests...)
}

func uploadParts(id string, body []byte) []controllerCommand {
	total := (len(body) + uploadPartMax - 1) / uploadPartMax
	out := make([]controllerCommand, 0, total)
	for i := range total {
		end := min((i+1)*uploadPartMax, len(body))
		out = append(out, part(id, i, total, body[i*uploadPartMax:end], http.MethodPost, "/rt/r1/attachments"))
	}
	return out
}

func TestTheHostAdvertisesUploadAndRunsTheWholeOnlyAfterTheLastPart(t *testing.T) {
	desktop := &recordingDesktop{}
	ch := newChannel(t, desktop)
	features, _ := ch.ready["features"].([]any)
	if !containsFeature(features, "upload") || !containsFeature(features, "replay") {
		t.Fatalf("features = %v, want replay and upload", features)
	}
	body := bytes.Repeat([]byte(`{"data":"abcdefghijklmnop"}`), 9000)
	parts := uploadParts("up-1", body)
	if len(parts) < 3 {
		t.Fatalf("fixture needs several parts, has %d", len(parts))
	}
	for i, p := range parts {
		ch.send(t, p, account.RemoteDesktop)
		if i < len(parts)-1 && len(desktop.seen()) != 0 {
			t.Fatalf("the desktop ran after part %d of %d", i, len(parts))
		}
	}
	got := desktop.seen()
	if len(got) != 1 || got[0].Method != http.MethodPost || got[0].Path != "/rt/r1/attachments" || !bytes.Equal(got[0].Body, body) {
		t.Fatalf("desktop saw %d requests; first %+v", len(got), got)
	}
	reply := readDirected(t, ch.controller, ch.cipher)
	if reply["type"] != "desktop.response" || reply["id"] != "up-1" || reply["done"] != true {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestAResentUploadAnswersFromTheKeptReplyAndRunsOnce(t *testing.T) {
	desktop := &recordingDesktop{}
	ch := newChannel(t, desktop)
	body := bytes.Repeat([]byte("p"), uploadPartMax*2+5)
	for round := range 2 {
		for _, p := range uploadParts("again", body) {
			ch.send(t, p, account.RemoteDesktop)
		}
		if reply := readDirected(t, ch.controller, ch.cipher); reply["type"] != "desktop.response" {
			t.Fatalf("round %d reply = %+v", round, reply)
		}
	}
	if n := len(desktop.seen()); n != 1 {
		t.Fatalf("a resent upload ran %d times", n)
	}
}

func TestAnUploadWithoutTheDesktopScopeIsRefusedAndHoldsNothing(t *testing.T) {
	desktop := &recordingDesktop{}
	ch := newChannel(t, desktop)
	parts := uploadParts("noscope", bytes.Repeat([]byte("q"), uploadPartMax*2))
	ch.send(t, parts[0])
	reply := readDirected(t, ch.controller, ch.cipher)
	if reply["type"] != "error" || len(desktop.seen()) != 0 {
		t.Fatalf("reply = %+v, desktop saw %d", reply, len(desktop.seen()))
	}
	if ch.sessions[ch.connectionID].uploads.current != nil {
		t.Fatal("a refused upload left bytes held")
	}
}

func TestAnOutOfOrderPartAbandonsTheUploadWithAnError(t *testing.T) {
	desktop := &recordingDesktop{}
	ch := newChannel(t, desktop)
	parts := uploadParts("order", bytes.Repeat([]byte("r"), uploadPartMax*3))
	ch.send(t, parts[0], account.RemoteDesktop)
	ch.send(t, parts[2], account.RemoteDesktop)
	if reply := readDirected(t, ch.controller, ch.cipher); reply["type"] != "error" {
		t.Fatalf("reply = %+v, want an error", reply)
	}
	ch.send(t, parts[1], account.RemoteDesktop)
	if reply := readDirected(t, ch.controller, ch.cipher); reply["type"] != "error" {
		t.Fatalf("a late part after the abandon reply = %+v, want an error", reply)
	}
	if len(desktop.seen()) != 0 {
		t.Fatal("an abandoned upload ran")
	}
}

func TestEndingTheControllerReleasesAHeldUploadAndStartsNothingToLeak(t *testing.T) {
	desktop := &recordingDesktop{}
	before := runtime.NumGoroutine()
	for range 20 {
		ch := newChannel(t, desktop)
		ch.send(t, uploadParts("held", bytes.Repeat([]byte("s"), uploadPartMax*3))[0], account.RemoteDesktop)
		if ch.sessions[ch.connectionID].uploads.current == nil {
			t.Fatal("fixture: the upload was not held")
		}
		if err := ch.host.handle(t.Context(), ch.deviceWire, &identity{DeviceID: ch.deviceID}, ch.device, ch.sessions, mustJSON(t, gatewayMessage{Type: "controller_disconnected", ConnectionID: ch.connectionID})); err != nil {
			t.Fatal(err)
		}
		if _, held := ch.sessions[ch.connectionID]; held {
			t.Fatal("the session, and the upload in it, outlived its controller")
		}
		ch.stop()
	}
	time.Sleep(200 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+4 {
		t.Fatalf("goroutines %d -> %d after twenty held uploads", before, after)
	}
}

func containsFeature(list []any, name string) bool {
	for _, item := range list {
		if fmt.Sprint(item) == name {
			return true
		}
	}
	return false
}
