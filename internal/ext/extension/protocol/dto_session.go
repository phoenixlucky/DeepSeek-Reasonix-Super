package protocol

// Session DTOs: a read-only view of the session, and a queued prompt, for an
// extension that declared the "session.read" or "session.submit" capability. Both ride Extension → Host
// requests; text an extension sends reaches the model only as data from it.

// SessionReadKind names what host/session/read returns.
type SessionReadKind string

const (
	SessionReadUsage    SessionReadKind = "usage"
	SessionReadMessages SessionReadKind = "messages"
)

// SessionReadParams asks for the last Limit transcript items (messages) or the
// last response's token counts (usage). Limit 0 takes the host's default.
type SessionReadParams struct {
	What  SessionReadKind `json:"what"`
	Limit int             `json:"limit,omitempty" validate:"min=0"`
}

// SessionUsage is the last response's token counts.
type SessionUsage struct {
	PromptTokens     int `json:"promptTokens" validate:"min=0"`
	CompletionTokens int `json:"completionTokens" validate:"min=0"`
	CachedTokens     int `json:"cachedTokens" validate:"min=0"`
}

// SessionMessage is one transcript item, credential-redacted and bounded.
type SessionMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// SessionReadResult carries Usage or Messages, by the kind asked for. Omitted
// counts the older items the host's ceiling left out.
type SessionReadResult struct {
	Usage    *SessionUsage    `json:"usage,omitempty"`
	Messages []SessionMessage `json:"messages,omitempty"`
	Omitted  int              `json:"omitted,omitempty" validate:"min=0"`
}

// PromptSubmitParams queues one prompt from the extension. The host labels it
// as the extension's own words and never as the user's.
type PromptSubmitParams struct {
	Text string `json:"text" validate:"nonempty"`
}

// PromptSubmitRefusedPerTurn is Refused when the user's per-turn ceiling on
// queued prompts is spent.
const PromptSubmitRefusedPerTurn = "submit_per_turn"

// PromptSubmitRefusedTooLong is Refused when one prompt is over the user's byte ceiling.
const PromptSubmitRefusedTooLong = "submit_bytes"

// PromptSubmitRefusedNoSession is Refused when the session has no saved path yet,
// so there is nowhere to queue.
const PromptSubmitRefusedNoSession = "no_session"

// PromptSubmitResult says whether the prompt is queued. A refusal the host chose
// is not a protocol error: Refused carries its identity.
type PromptSubmitResult struct {
	Queued  bool   `json:"queued"`
	Refused string `json:"refused,omitempty"`
}
