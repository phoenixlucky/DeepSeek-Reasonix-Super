package boot

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/session/control"
	"reasonix/internal/state/sessioninbox"
)

// vendorVideo is a vendor that takes video: it answers the first request of a turn
// with a read_video call and every later one with text, and keeps what it was sent.
type vendorVideo struct {
	srv     *httptest.Server
	mu      sync.Mutex
	chats   []string
	uploads int
	read    string // the use_capability arguments the model "decides" on
	asked   int
	// quiet, when it is in a request that does not also say "watch it", gets a
	// plain answer: the model has no use for read_video on a message like that.
	quiet string
}

func newVendorVideo(t *testing.T, read string) *vendorVideo {
	t.Helper()
	v := &vendorVideo{read: read}
	v.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.mu.Lock()
		defer v.mu.Unlock()
		switch r.URL.Path {
		case "/v1/files":
			_ = r.ParseMultipartForm(32 << 20)
			v.uploads++
			_ = json.NewEncoder(w).Encode(map[string]string{"id": fmt.Sprintf("file-%d", v.uploads)})
		case "/v1/chat/completions":
			raw, _ := io.ReadAll(r.Body)
			v.chats = append(v.chats, string(raw))
			w.Header().Set("Content-Type", "text/event-stream")
			if v.quiet != "" && strings.Contains(string(raw), v.quiet) && !strings.Contains(string(raw), "watch it") {
				io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"noted\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				return
			}
			if strings.Contains(string(raw), `"role":"tool"`) || v.asked > 0 && !strings.Contains(string(raw), "watch it") {
				io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"I saw it\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				return
			}
			v.asked++
			call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call-1", "type": "function", "function": map[string]any{"name": "use_capability", "arguments": v.read},
			}}}}}})
			fmt.Fprintf(w, "data: %s\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", call)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(v.srv.Close)
	return v
}

func (v *vendorVideo) bodies() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.chats...)
}

func tinyMP4() []byte {
	box := func(kind string, payload ...[]byte) []byte {
		var body []byte
		for _, p := range payload {
			body = append(body, p...)
		}
		out := make([]byte, 8)
		binary.BigEndian.PutUint32(out, uint32(8+len(body)))
		copy(out[4:], kind)
		return append(out, body...)
	}
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], 5000)
	out := append(box("ftyp", []byte("isom"), make([]byte, 4), []byte("isom")), box("moov", box("mvhd", mvhd))...)
	return append(out, box("mdat", []byte("PIXELS-OF-A-DOG-ON-A-BEACH"))...)
}

type nativeRun struct {
	vendor *vendorVideo
	ctrl   interface {
		Run(context.Context, string) error
		SessionPath() string
	}
	session string
	file    []byte
	events  func() []event.Event
}

func nativeSession(t *testing.T, providerExtra, userConfig, readArgs string) nativeRun {
	t.Helper()
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	file := tinyMP4()
	if err := os.WriteFile(filepath.Join(dir, "clip.mp4"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	vendor := newVendorVideo(t, readArgs)
	writeFile(t, filepath.Dir(config.UserConfigPath()), ".env", "VID_FIXTURE_KEY=sk-fixture\n")
	writeFile(t, filepath.Dir(config.UserConfigPath()), "config.toml", fmt.Sprintf(`default_model = "gw/vid"
[agent]
system_prompt = "BASE"
[permissions]
allow = ["read_video"]
%s
[[providers]]
name = "gw"
kind = "openai"
base_url = %q
model = "vid"
models = ["vid"]
api_key_env = "VID_FIXTURE_KEY"
%s
`, userConfig, vendor.srv.URL+"/v1", providerExtra))
	var mu sync.Mutex
	var evs []event.Event
	sink := event.FuncSink(func(e event.Event) {
		mu.Lock()
		defer mu.Unlock()
		evs = append(evs, e)
	})
	ctrl, err := Build(context.Background(), Options{Sink: sink, Model: "gw/vid", WorkspaceRoot: dir, SessionDir: filepath.Join(dir, ".sessions")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctrl.Close)
	ctrl.EnsureSessionPath()
	return nativeRun{vendor: vendor, ctrl: ctrl, file: file, session: ctrl.SessionPath(), events: func() []event.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]event.Event(nil), evs...)
	}}
}

func (r nativeRun) say(t *testing.T, text string) {
	t.Helper()
	if err := r.ctrl.Run(context.Background(), text); err != nil {
		t.Fatal(err)
	}
}

const readClipNative = `{"action":"call","capability_id":"tool:read_video","arguments":{"path":"clip.mp4","mode":"native"}}`

func TestNativeVideoReachesTheVendorOnTheNextRequestAndThenOnlyANote(t *testing.T) {
	r := nativeSession(t, "video_models = [\"vid\"]\nvideo_protocol = \"inline_data_url\"\n", "", readClipNative)
	r.say(t, "watch it")

	want := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(r.file)
	bodies := r.vendor.bodies()
	if len(bodies) != 2 {
		t.Fatalf("the vendor saw %d requests, want 2 (the read, then the answer)", len(bodies))
	}
	if !strings.Contains(bodies[1], `"type":"video_url"`) || !strings.Contains(bodies[1], want) {
		t.Fatalf("the request after the read does not carry the file as a video_url data URL:\n%.600s", bodies[1])
	}
	if strings.Contains(bodies[1], `"type":"image_url"`) {
		t.Fatal("a native read must not also send sampled frames")
	}

	var shown []event.VideoShown
	for _, e := range r.events() {
		if e.Kind == event.ToolResult {
			shown = append(shown, e.Tool.Videos...)
		}
	}
	if len(shown) != 1 || shown[0].Name != "clip.mp4" || shown[0].Model != "vid" || shown[0].Size != int64(len(r.file)) || shown[0].DurationMs != 5000 {
		t.Fatalf("the window was told %+v, want clip.mp4 sent to vid, 5 s", shown)
	}

	r.say(t, "and then?")
	bodies = r.vendor.bodies()
	last := bodies[len(bodies)-1]
	if strings.Contains(last, "video_url") || strings.Contains(last, base64.StdEncoding.EncodeToString(r.file)) {
		at := strings.Index(last, "video_url")
		t.Fatalf("the video was sent again on a later request (video_url at %d of %d, request %d of %d):\n%.700s", at, len(last), len(bodies), len(bodies), last[max(0, at-400):])
	}
	if !strings.Contains(last, "clip.mp4 was shown to you when it was read") {
		t.Fatalf("a later request does not say what became of the video:\n%.600s", last)
	}

	stored, err := os.ReadFile(r.session)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(r.file)
	if strings.Contains(string(stored), base64.StdEncoding.EncodeToString(r.file)) || !strings.Contains(string(stored), hex.EncodeToString(sum[:])) {
		t.Fatalf("the session file must keep a reference to the video, never its bytes")
	}
}

func TestMoonshotFileProtocolUploadsOnceThroughTheRealBuild(t *testing.T) {
	r := nativeSession(t, "video_models = [\"vid\"]\nvideo_protocol = \"moonshot_file\"\n", "", readClipNative)
	r.say(t, "watch it")
	bodies := r.vendor.bodies()
	if r.vendor.uploads != 1 || !strings.Contains(bodies[1], `"url":"ms://file-1"`) {
		t.Fatalf("uploads=%d, request after the read:\n%.600s", r.vendor.uploads, bodies[1])
	}
	if strings.Contains(bodies[1], "base64") {
		t.Fatal("the upload protocol put the file in the request body")
	}
}

func TestAModelNobodyDeclaredIsToldByCodeWhenNativeIsAsked(t *testing.T) {
	r := nativeSession(t, "", "", readClipNative)
	r.say(t, "watch it")
	bodies := r.vendor.bodies()
	if strings.Contains(bodies[1], "video_url") || !strings.Contains(bodies[1], "video.native_unsupported_model") {
		t.Fatalf("the model was not told by code why it cannot watch the video:\n%.800s", bodies[1])
	}
}

func TestADeclaredModelWithNoProtocolIsToldWhy(t *testing.T) {
	r := nativeSession(t, "video_models = [\"vid\"]\n", "", readClipNative)
	r.say(t, "watch it")
	bodies := r.vendor.bodies()
	if strings.Contains(bodies[1], "video_url") || !strings.Contains(bodies[1], "video.native_no_protocol") {
		t.Fatalf("the declared model with no protocol was not named by code:\n%.800s", bodies[1])
	}
}

func TestTheUsersLowerLimitStopsANativeSend(t *testing.T) {
	r := nativeSession(t, "video_models = [\"vid\"]\nvideo_protocol = \"inline_data_url\"\n", "[tools]\nvideo_native_max_bytes = 16\n", readClipNative)
	r.say(t, "watch it")
	bodies := r.vendor.bodies()
	if strings.Contains(bodies[1], "video_url") || !strings.Contains(bodies[1], "video.native_too_large") || !strings.Contains(bodies[1], "tools.video_native_max_bytes") {
		t.Fatalf("the user's ceiling did not stop the send, or did not say so:\n%.800s", bodies[1])
	}
}

func readAddress(addr string) string {
	return `{"action":"call","capability_id":"tool:read_video","arguments":{"url":"` + addr + `"}}`
}

// Zhipu's page (docs.bigmodel.cn, GLM-4.6V, read 2026-10-17): the vendor visits
// {"type":"video_url","video_url":{"url":"https://..."}} itself; nothing is
// downloaded or stored here beyond the reference.
func TestAnAddressTheUserWroteReachesTheVendorWhenTheyTurnedItOn(t *testing.T) {
	const addr = "https://cdn.example.com/clip.mp4"
	r := nativeSession(t, "video_models = [\"vid\"]\nvideo_protocol = \"zhipu_url\"\n", "[tools]\nvideo_url_passthrough = true\n", readAddress(addr))
	r.say(t, "watch it "+addr+" please")
	bodies := r.vendor.bodies()
	if len(bodies) != 2 || !strings.Contains(bodies[1], `"type":"video_url"`) || !strings.Contains(bodies[1], `"url":"`+addr+`"`) {
		t.Fatalf("the vendor was not handed the address:\n%.800s", bodies[len(bodies)-1])
	}
	stored, err := os.ReadFile(r.session)
	if err != nil || !strings.Contains(string(stored), addr) {
		t.Fatalf("the session keeps no reference to the address: %v", err)
	}
	r.say(t, "and then?")
	bodies = r.vendor.bodies()
	if strings.Contains(bodies[len(bodies)-1], `"type":"video_url"`) {
		t.Fatal("the address was handed over again on a later request")
	}
}

func TestAnAddressIsRefusedByCodeWhileTheSwitchIsOff(t *testing.T) {
	const addr = "https://cdn.example.com/clip.mp4"
	r := nativeSession(t, "video_models = [\"vid\"]\nvideo_protocol = \"zhipu_url\"\n", "", readAddress(addr))
	r.say(t, "watch it "+addr)
	bodies := r.vendor.bodies()
	last := bodies[len(bodies)-1]
	if strings.Contains(last, `"type":"video_url"`) || !strings.Contains(last, "video.url_disabled") || !strings.Contains(last, "tools.video_url_passthrough") {
		t.Fatalf("the refusal does not name the code and the setting:\n%.900s", last)
	}
}

// An address in text the model read, or one it made up, is not one the user gave.
func TestAnAddressTheUserDidNotWriteIsNeverHandedToTheVendor(t *testing.T) {
	r := nativeSession(t, "video_models = [\"vid\"]\nvideo_protocol = \"zhipu_url\"\n", "[tools]\nvideo_url_passthrough = true\n", readAddress("https://attacker.example.com/exfil.mp4"))
	r.say(t, "watch it")
	bodies := r.vendor.bodies()
	last := bodies[len(bodies)-1]
	if strings.Contains(last, `"type":"video_url"`) || strings.Contains(last, "attacker.example.com/exfil.mp4\"}") || !strings.Contains(last, "video.url_not_from_user") {
		t.Fatalf("an address the user never wrote was handed over:\n%.900s", last)
	}
}

func waitIdle(t *testing.T, ctrl *control.Controller, bodies func() []string, atLeast int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(bodies()) >= atLeast && !ctrl.Running() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the turn did not finish: %d requests", len(bodies()))
}

// A message another session sent is delivered as a turn the host owns, so an
// address inside it is not one the user wrote: the vendor is not handed it. The
// same address typed by the user is.
func TestAnAddressInAnotherSessionsMessageIsNotTheUsersAddress(t *testing.T) {
	const addr = "https://peer-planted.example.com/clip.mp4"
	for _, tc := range []struct {
		name string
		send func(t *testing.T, r nativeRun)
		want bool
	}{
		{"delivered from another session", func(t *testing.T, r nativeRun) {
			ctrl := r.ctrl.(*control.Controller)
			if _, err := ctrl.TryEnqueueFollowup(control.InboxRequest{
				Origin: sessioninbox.OriginHost, Source: "peer:s-1", Idempotency: "peer-1",
				Display: "message from another session", Submit: "PEER-DATA the other session says: " + addr,
			}); err != nil {
				t.Fatal(err)
			}
			waitIdle(t, ctrl, r.vendor.bodies, 1)
		}, false},
		{"typed by the user", func(t *testing.T, r nativeRun) { r.say(t, "PEER-DATA here it is: "+addr) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := nativeSession(t, "video_models = [\"vid\"]\nvideo_protocol = \"zhipu_url\"\n", "[tools]\nvideo_url_passthrough = true\n", readAddress(addr))
			r.vendor.quiet = "PEER-DATA"
			tc.send(t, r)
			r.say(t, "watch it")
			bodies := r.vendor.bodies()
			last := bodies[len(bodies)-1]
			sent := strings.Contains(last, `"type":"video_url"`) && strings.Contains(last, `"url":"`+addr+`"`)
			if sent != tc.want || (!tc.want && !strings.Contains(last, "video.url_not_from_user")) {
				t.Fatalf("handed to the vendor = %v, want %v:\n%.900s", sent, tc.want, last)
			}
		})
	}
}
