package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"reasonix/internal/contract/provider"
)

// A gateway that accepts the request and then goes silent must not keep Stop
// from finishing: cancelling closes the stream within a bounded time at every
// phase the server can hang in.
func TestCancelEndsAStreamWhereverTheGatewayGoesSilent(t *testing.T) {
	for name, serve := range map[string]http.HandlerFunc{
		"before headers": func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() },
		"after headers": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		},
		"mid stream": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		},
		"keepalive comments only": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			for r.Context().Err() == nil {
				_, _ = io.WriteString(w, ": keepalive\n\n")
				w.(http.Flusher).Flush()
				time.Sleep(20 * time.Millisecond)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			// The server notices a closed connection only after the request
			// body has been read.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				serve(w, r)
			}))
			defer srv.Close()
			p, err := New(provider.Config{Name: "gateway", BaseURL: srv.URL, Model: "m", APIKey: "k"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(150*time.Millisecond, cancel)
			done := make(chan struct{})
			go func() {
				defer close(done)
				ch, err := p.Stream(ctx, provider.Request{Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}}})
				if err != nil {
					return
				}
				for range ch {
				}
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("the stream was still open 3s after cancel")
			}
		})
	}
}
