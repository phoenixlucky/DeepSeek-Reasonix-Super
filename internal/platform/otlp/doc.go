// Package otlp exports a run as OpenTelemetry GenAI traces over OTLP/HTTP JSON.
//
// It is an event.Sink decorator: structure, timing, token counts, model and tool
// names, and failure classes leave the machine; prompts, tool arguments, results,
// file paths and error text never do. Spans are built from typed event fields
// through one fixed attribute set (attrs.go); nothing copies free text onto one.
//
// The conventions are marked Development upstream (read 2026-10-18), so the
// mapping lives in sink.go and attrs.go only, and every span says so through
// the reasonix.semconv_experimental resource attribute.
//
// Export never blocks a turn: spans go into a bounded queue and a full queue or
// a failed send drops them and counts it. A client never follows redirects, so
// the credential header goes only to the configured host.
package otlp
