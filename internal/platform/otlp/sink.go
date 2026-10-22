package otlp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"reasonix/internal/contract/event"
)

// Sink turns the event stream into spans: one trace per turn (invoke_agent),
// a chat span per sampling attempt, an execute_tool span per tool call. It
// forwards every event unchanged.
type Sink struct {
	event.AuditForwarder
	exp   *Exporter
	agent Safe
	now   func() time.Time

	mu    sync.Mutex
	turn  *open
	chats map[string]*open
	tools map[string]*open
}

type open struct {
	span  Span
	model string
}

// NewSink wraps inner. A nil exporter returns inner untouched.
func NewSink(inner event.Sink, exp *Exporter, agent string) event.Sink {
	if exp == nil {
		return inner
	}
	s := &Sink{AuditForwarder: event.AuditForwarder{Inner: inner}, exp: exp, agent: Name(agent), now: time.Now,
		chats: map[string]*open{}, tools: map[string]*open{}}
	exp.beforeClose(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.endTurn(event.Event{Kind: event.TurnDone, Cancelled: true})
	})
	return s
}

// RecordTurnCompletion ends the turn of a synchronous run, which emits no
// TurnDone, then passes the signal inward.
func (s *Sink) RecordTurnCompletion() {
	s.mu.Lock()
	s.endTurn(event.Event{Kind: event.TurnDone})
	s.mu.Unlock()
	s.AuditForwarder.RecordTurnCompletion()
}

// Emit forwards e, then records what it says about the run.
func (s *Sink) Emit(e event.Event) {
	if s.Inner != nil {
		s.Inner.Emit(e)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch e.Kind {
	case event.TurnStarted:
		s.startTurn(e)
	case event.StreamAttempt:
		s.attempt(e)
	case event.Usage:
		s.usage(e)
	case event.ToolDispatch:
		s.toolStart(e.Tool)
	case event.ToolResult:
		s.toolEnd(e.Tool)
	case event.TurnDone:
		s.endTurn(e)
	}
}

// Flush sends what is queued and passes the request inward when the inner sink
// can flush too.
func (s *Sink) Flush(ctx context.Context) error {
	err := s.exp.Flush(ctx)
	if f, ok := s.Inner.(interface{ Flush(context.Context) error }); ok {
		if ierr := f.Flush(ctx); err == nil {
			err = ierr
		}
	}
	return err
}

func (s *Sink) stamp() int64 { return s.now().UnixNano() }

func (s *Sink) child(name string, kind int, parent string) *open {
	o := &open{span: Span{Name: name, Kind: kind, SpanID: randHex(8), ParentID: parent, StartNano: s.stamp()}}
	if s.turn != nil {
		o.span.TraceID = s.turn.span.TraceID
		if parent == "" {
			o.span.ParentID = s.turn.span.SpanID
		}
	} else {
		o.span.TraceID = randHex(16)
	}
	return o
}

// finish enqueues o, ending it now unless it already ended.
func (s *Sink) finish(o *open, failed bool, attrs ...Attr) {
	if o.span.EndNano == 0 {
		o.span.EndNano = max(s.stamp(), o.span.StartNano)
	}
	o.span.Failed = o.span.Failed || failed
	o.span.Attrs = append(o.span.Attrs, attrs...)
	s.exp.Enqueue(o.span)
}

func (s *Sink) startTurn(e event.Event) {
	s.endTurn(event.Event{Kind: event.TurnDone, Cancelled: true})
	model := Name(e.ModelRef)
	o := &open{model: string(model), span: Span{Name: "invoke_agent " + string(s.agent), Kind: kindInternal,
		TraceID: randHex(16), SpanID: randHex(8), StartNano: s.stamp()}}
	o.span.Attrs = []Attr{Str("gen_ai.operation.name", "invoke_agent"), Str("gen_ai.agent.name", s.agent)}
	if e.ModelRef != "" {
		o.span.Attrs = append(o.span.Attrs, Str("gen_ai.request.model", model))
	}
	s.turn = o
}

func (s *Sink) endTurn(e event.Event) {
	if s.turn == nil {
		return
	}
	for id, o := range s.chats {
		s.finish(o, false)
		delete(s.chats, id)
	}
	for id, o := range s.tools {
		s.finish(o, true, Str("error.type", "incomplete"))
		delete(s.tools, id)
	}
	failed, class := false, Safe("")
	switch {
	case e.Cancelled:
		failed, class = true, "cancelled"
	case e.Err != nil:
		failed, class = true, "error"
	}
	var attrs []Attr
	if class != "" {
		attrs = append(attrs, Str("error.type", class))
	}
	if e.Outcome != "" {
		attrs = append(attrs, Str("reasonix.turn.outcome", Name(e.Outcome)))
	}
	s.finish(s.turn, failed, attrs...)
	s.turn = nil
	s.flushSoon()
}

func (s *Sink) flushSoon() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		defer cancel()
		_ = s.exp.Flush(ctx)
	}()
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func providerOf(ref string) Safe {
	if i := strings.IndexByte(ref, '/'); i > 0 {
		return Name(ref[:i])
	}
	return "unknown"
}
