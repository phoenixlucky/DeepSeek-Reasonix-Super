package otlp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	queueCap     = 512
	batchSize    = 64
	batchEvery   = 2 * time.Second
	sendTimeout  = 5 * time.Second
	maxErrorBody = 1 << 10
)

// Stats counts what the exporter did with the spans it was given.
type Stats struct{ Sent, Dropped, Failed int64 }

// Exporter posts finished spans to one collector. Enqueue never blocks.
type Exporter struct {
	client   *http.Client
	url      string
	hdrName  string
	hdrValue string
	resource []Attr

	queue chan Span
	kick  chan chan struct{}
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once

	sent, dropped, failed atomic.Int64

	mu       sync.Mutex
	finalize []func()
}

// New validates the settings and starts the worker. The client's redirects are
// disabled on a copy, so the credential header goes only to the configured host.
func New(s Settings, client *http.Client) (*Exporter, error) {
	target, err := s.target()
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{}
	}
	own := *client
	own.Timeout = sendTimeout
	own.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	service := s.ServiceName
	if service == "" {
		service = "reasonix"
	}
	e := &Exporter{client: &own, url: target, hdrName: s.HeaderName, hdrValue: s.HeaderValue,
		resource: []Attr{Str("service.name", Name(service)), Str("service.version", Name(s.Version)), Bool("reasonix.semconv_experimental", true)},
		queue:    make(chan Span, queueCap), kick: make(chan chan struct{}), stop: make(chan struct{}), done: make(chan struct{})}
	go e.run()
	return e, nil
}

// Enqueue hands a finished span to the worker, or drops it when the queue is full.
func (e *Exporter) Enqueue(s Span) {
	select {
	case e.queue <- s:
	default:
		e.dropped.Add(1)
	}
}

// Stats reports the counts so far.
func (e *Exporter) Stats() Stats {
	return Stats{Sent: e.sent.Load(), Dropped: e.dropped.Load(), Failed: e.failed.Load()}
}

// Flush sends what is queued and waits for it, up to ctx.
func (e *Exporter) Flush(ctx context.Context) error {
	ack := make(chan struct{})
	select {
	case e.kick <- ack:
	case <-e.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// beforeClose registers work that must reach the queue before the final flush,
// such as a turn still open when the session ends.
func (e *Exporter) beforeClose(f func()) {
	e.mu.Lock()
	e.finalize = append(e.finalize, f)
	e.mu.Unlock()
}

// Close flushes, then stops the worker.
func (e *Exporter) Close(ctx context.Context) error {
	e.mu.Lock()
	fs := e.finalize
	e.finalize = nil
	e.mu.Unlock()
	for _, f := range fs {
		f()
	}
	err := e.Flush(ctx)
	e.once.Do(func() { close(e.stop) })
	select {
	case <-e.done:
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
	return err
}

func (e *Exporter) run() {
	defer close(e.done)
	tick := time.NewTicker(batchEvery)
	defer tick.Stop()
	var batch []Span
	send := func() {
		if len(batch) > 0 {
			e.post(batch)
			batch = nil
		}
	}
	drain := func() {
		for {
			select {
			case s := <-e.queue:
				batch = append(batch, s)
				if len(batch) >= batchSize {
					send()
				}
			default:
				return
			}
		}
	}
	for {
		select {
		case s := <-e.queue:
			batch = append(batch, s)
			if len(batch) >= batchSize {
				send()
			}
		case <-tick.C:
			send()
		case ack := <-e.kick:
			drain()
			send()
			close(ack)
		case <-e.stop:
			drain()
			send()
			return
		}
	}
}

func (e *Exporter) post(spans []Span) {
	body, err := encodeBatch(e.resource, spans)
	if err == nil {
		err = e.send(body)
	}
	if err != nil {
		n := e.failed.Add(int64(len(spans)))
		if n == int64(len(spans)) {
			slog.Warn("otlp: export failed; spans are dropped", "endpoint", e.url, "err", err)
		}
		return
	}
	e.sent.Add(int64(len(spans)))
}

func (e *Exporter) send(body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.hdrName != "" && e.hdrValue != "" {
		req.Header.Set(e.hdrName, e.hdrValue)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return redact(err, e.hdrValue)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errors.New("collector answered " + resp.Status)
	}
	return nil
}

// redact keeps a transport error from carrying the credential into a log line.
func redact(err error, secret string) error {
	if secret == "" || !bytes.Contains([]byte(err.Error()), []byte(secret)) {
		return err
	}
	return errors.New("request failed (details withheld: they contained the credential)")
}
