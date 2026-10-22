package otlp

import (
	"strings"

	"reasonix/internal/contract/event"
)

var finishReasons = []string{"stop", "tool_calls", "length", "content_filter", "repetition_truncation", "interrupted"}

var usageSources = []string{"executor", "planner", "advisor", "best-of", "best-of-judge", "compaction", "subagent", "side"}

func (s *Sink) attempt(e event.Event) {
	info := e.StreamAttempt
	switch info.Action {
	case event.StreamAttemptBegin:
		o := s.child("chat", kindClient, "")
		model := s.modelOf(e.ModelRef)
		o.model = model
		o.span.Name = "chat " + model
		o.span.Attrs = []Attr{Str("gen_ai.operation.name", "chat"), Str("gen_ai.provider.name", providerOf(model)),
			Str("gen_ai.request.model", Name(model)), Int("reasonix.attempt", int64(info.Attempt))}
		s.chats[info.ID] = o
	case event.StreamAttemptCommit, event.StreamAttemptDiscard:
		o := s.chats[info.ID]
		if o == nil {
			return
		}
		o.span.EndNano = s.stamp()
		outcome := Safe("commit")
		if info.Action == event.StreamAttemptDiscard {
			outcome = "discard"
			o.span.Failed = true
		}
		o.span.Attrs = append(o.span.Attrs, Str("reasonix.attempt.outcome", outcome))
		if info.FirstTokenMs > 0 {
			o.span.Attrs = append(o.span.Attrs, Int("reasonix.first_token_ms", info.FirstTokenMs))
		}
	}
}

func (s *Sink) modelOf(ref string) string {
	if ref != "" {
		return string(Name(ref))
	}
	if s.turn != nil {
		return s.turn.model
	}
	return "unknown"
}

func (s *Sink) usage(e event.Event) {
	u := e.Usage
	if u == nil {
		return
	}
	attrs := []Attr{Int("gen_ai.usage.input_tokens", int64(u.PromptTokens)), Int("gen_ai.usage.output_tokens", int64(u.CompletionTokens)),
		Int("gen_ai.usage.cache_read.input_tokens", int64(u.CacheHitTokens))}
	if u.FinishReason != "" {
		attrs = append(attrs, Str("gen_ai.response.finish_reasons", OneOf(u.FinishReason, finishReasons...)))
	}
	if o := s.chats[e.AttemptID]; o != nil && e.AttemptID != "" {
		delete(s.chats, e.AttemptID)
		s.finish(o, false, attrs...)
		return
	}
	model := s.modelOf(e.ModelRef)
	o := s.child("chat "+model, kindClient, "")
	o.span.Attrs = append([]Attr{Str("gen_ai.operation.name", "chat"), Str("gen_ai.provider.name", providerOf(model)),
		Str("gen_ai.request.model", Name(model)), Str("reasonix.usage.source", OneOf(e.UsageSource, usageSources...))}, attrs...)
	s.finish(o, false)
}

func (s *Sink) toolStart(t event.Tool) {
	if t.Partial || t.ID == "" || s.tools[t.ID] != nil {
		return
	}
	name := Name(t.Name)
	parent := ""
	if p := s.tools[t.ParentID]; p != nil && t.ParentID != "" {
		parent = p.span.SpanID
	}
	o := s.child("execute_tool "+string(name), kindInternal, parent)
	o.span.Attrs = []Attr{Str("gen_ai.operation.name", "execute_tool"), Str("gen_ai.tool.name", name),
		Str("gen_ai.tool.call.id", Name(t.ID)), Str("gen_ai.tool.type", toolType(t.Name))}
	s.tools[t.ID] = o
}

func toolType(name string) Safe {
	if strings.HasPrefix(name, "mcp__") {
		return "extension"
	}
	return "function"
}

func (s *Sink) toolEnd(t event.Tool) {
	o := s.tools[t.ID]
	if o == nil {
		s.toolStart(event.Tool{ID: t.ID, Name: t.Name, ParentID: t.ParentID})
		o = s.tools[t.ID]
		if o == nil {
			return
		}
	}
	delete(s.tools, t.ID)
	if t.StartedAt > 0 {
		o.span.StartNano = t.StartedAt * 1_000_000
	}
	if t.EndedAt > 0 {
		o.span.EndNano = max(t.EndedAt*1_000_000, o.span.StartNano)
	}
	attrs := []Attr{Bool("reasonix.tool.read_only", t.ReadOnly)}
	failed := t.Err != "" || t.RefusalCode != ""
	if failed {
		attrs = append(attrs, Str("error.type", errorClass(t.RefusalCode)))
	}
	s.finish(o, failed, attrs...)
}

// errorClass is the host's own identity for a refusal when it has one: a dotted
// code the host minted, never the error's wording.
func errorClass(code string) Safe {
	if code != "" {
		return Name(code)
	}
	return "tool_error"
}
