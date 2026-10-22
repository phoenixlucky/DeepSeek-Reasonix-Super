// Command relayfake stands in for the two services between a hosted page and a
// Studio: the accounts API that registers the Studio and mints a controller's
// grant, and the message relay that carries ciphertext between them. It
// understands routing and nothing else, so everything above it — the page's
// handshake, the encrypted channel, the Studio's host — is the real code.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type device struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Platform     string   `json:"platform"`
	PublicKey    string   `json:"publicKey"`
	Capabilities []string `json:"capabilities"`
	RevokedAt    *string  `json:"revokedAt"`
}

type controller struct {
	id   string
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *controller) write(kind int, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(kind, payload)
}

type fake struct {
	mu          sync.Mutex
	device      *device
	credential  string
	tickets     map[string]bool
	hostConn    *websocket.Conn
	hostWrite   sync.Mutex
	controllers map[string]*controller
	upgrader    websocket.Upgrader
}

func main() {
	f := &fake{
		tickets:     map[string]bool{},
		controllers: map[string]*controller{},
		upgrader:    websocket.Upgrader{Subprotocols: []string{"reasonix.remote.v1"}, CheckOrigin: func(*http.Request) bool { return true }},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /me", f.me)
	mux.HandleFunc("GET /me/devices", f.listDevices)
	mux.HandleFunc("POST /me/devices", f.registerDevice)
	mux.HandleFunc("OPTIONS /me/remote-grants", f.cors(func(http.ResponseWriter, *http.Request) {}))
	mux.HandleFunc("POST /me/remote-grants", f.cors(f.grant))
	mux.HandleFunc("GET /v1/devices/{id}/connect", f.deviceConnect)
	mux.HandleFunc("GET /v1/sessions/connect", f.controllerConnect)
	mux.HandleFunc("POST /ctl/drop-controllers", f.dropControllers)
	mux.HandleFunc("POST /ctl/drop-device", f.dropDevice)
	mux.HandleFunc("GET /ctl/state", f.state)
	addr := listener.Addr().String()
	out, _ := json.Marshal(map[string]string{"accounts": "http://" + addr, "relay": "ws://" + addr})
	fmt.Println(string(out))
	if err := http.Serve(listener, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func token() string {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fake) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "content-type")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func (f *fake) me(w http.ResponseWriter, _ *http.Request) {
	reply(w, http.StatusOK, map[string]any{"user": map[string]any{"id": 1, "handle": "fixture", "email": "fixture@example.test", "emailVerified": true}})
}

func (f *fake) listDevices(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	devices := []*device{}
	if f.device != nil {
		devices = append(devices, f.device)
	}
	reply(w, http.StatusOK, map[string]any{"devices": devices})
}

func (f *fake) registerDevice(w http.ResponseWriter, r *http.Request) {
	var in device
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.PublicKey == "" {
		reply(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "bad_request"}})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	in.ID = token() + token()
	f.device, f.credential = &in, token()
	reply(w, http.StatusOK, map[string]any{"device": f.device, "deviceCredential": f.credential})
}

func (f *fake) grant(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TargetDeviceID string `json:"targetDeviceId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.device == nil || f.device.ID != in.TargetDeviceID {
		reply(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "device_not_found"}})
		return
	}
	ticket := token()
	f.tickets[ticket] = true
	reply(w, http.StatusOK, map[string]any{"grant": map[string]string{"ticket": ticket}, "device": f.device})
}

func (f *fake) deviceConnect(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	known := f.device != nil && f.device.ID == r.PathValue("id") && r.Header.Get("Authorization") == "Bearer "+f.credential
	f.mu.Unlock()
	if !known {
		http.Error(w, "unknown device", http.StatusUnauthorized)
		return
	}
	conn, err := f.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.hostConn = conn
	f.mu.Unlock()
	f.hello()
	defer func() {
		f.mu.Lock()
		if f.hostConn == conn {
			f.hostConn = nil
		}
		f.mu.Unlock()
		conn.Close()
	}()
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var directed struct {
			To      string `json:"to"`
			Payload string `json:"payload"`
		}
		if json.Unmarshal(payload, &directed) != nil || directed.Payload == "" {
			continue
		}
		f.mu.Lock()
		target := f.controllers[directed.To]
		f.mu.Unlock()
		if target != nil {
			_ = target.write(websocket.TextMessage, []byte(directed.Payload))
		}
	}
}

func (f *fake) toHost(message map[string]any) {
	wire, _ := json.Marshal(message)
	f.mu.Lock()
	conn := f.hostConn
	f.mu.Unlock()
	if conn == nil {
		return
	}
	f.hostWrite.Lock()
	defer f.hostWrite.Unlock()
	_ = conn.WriteMessage(websocket.TextMessage, wire)
}

func (f *fake) hello() {
	f.mu.Lock()
	ids := make([]string, 0, len(f.controllers))
	for id := range f.controllers {
		ids = append(ids, id)
	}
	f.mu.Unlock()
	f.toHost(map[string]any{"type": "relay_hello", "controllers": ids, "heartbeatMs": 20000})
}

func (f *fake) controllerConnect(w http.ResponseWriter, r *http.Request) {
	ticket := ""
	for _, offered := range websocket.Subprotocols(r) {
		if rest, ok := strings.CutPrefix(offered, "reasonix.auth."); ok {
			ticket = rest
		}
	}
	f.mu.Lock()
	granted, online := f.tickets[ticket] || r.URL.Query().Get("fixture") == "1", f.hostConn != nil
	f.mu.Unlock()
	if !granted || !online {
		http.Error(w, "no grant or device offline", http.StatusServiceUnavailable)
		return
	}
	conn, err := f.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &controller{id: token(), conn: conn}
	f.mu.Lock()
	f.controllers[c.id] = c
	f.mu.Unlock()
	f.toHost(map[string]any{"type": "controller_connected", "connectionId": c.id})
	f.hello()
	defer func() {
		f.mu.Lock()
		delete(f.controllers, c.id)
		f.mu.Unlock()
		conn.Close()
		f.toHost(map[string]any{"type": "controller_disconnected", "connectionId": c.id})
		f.hello()
	}()
	for {
		kind, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if kind != websocket.TextMessage {
			continue
		}
		f.toHost(map[string]any{
			"type": "controller_message", "connectionId": c.id,
			"scopes": []string{"desktop"}, "payload": string(payload),
		})
	}
}

func (f *fake) dropControllers(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	all := make([]*controller, 0, len(f.controllers))
	for _, c := range f.controllers {
		all = append(all, c)
	}
	f.mu.Unlock()
	for _, c := range all {
		_ = c.conn.Close()
	}
	reply(w, http.StatusOK, map[string]int{"dropped": len(all)})
}

func (f *fake) dropDevice(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	conn := f.hostConn
	f.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	reply(w, http.StatusOK, map[string]bool{"dropped": conn != nil})
}

func (f *fake) state(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := ""
	if f.device != nil {
		id = f.device.ID
	}
	reply(w, http.StatusOK, map[string]any{"deviceId": id, "deviceOnline": f.hostConn != nil, "controllers": len(f.controllers)})
}
