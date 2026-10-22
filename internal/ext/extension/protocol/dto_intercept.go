package protocol

import "encoding/json"

// Intercept DTOs: the blocking hook call. The host waits up to
// TimeoutMillis; a late answer is an intercept_timeout error and the host
// proceeds with its default behavior.

// InterceptParams asks the extension to rule on one intercepted event. Seq
// is the host's monotonic intercept sequence for this session so both peers
// can detect drops and reordering. When Payload exceeds ExternalizeFieldBytes
// it travels as null and Externalized carries its content-ref descriptor.
type InterceptParams struct {
	Event         InterceptEvent      `json:"event"`
	Seq           uint64              `json:"seq" validate:"min=1"`
	Payload       json.RawMessage     `json:"payload" externalizable:"true"`
	TimeoutMillis int                 `json:"timeoutMillis" validate:"min=0"`
	Externalized  []ExternalizedField `json:"externalized,omitempty"`
}

// InterceptResult is the extension's ruling. Replacement carries the new payload
// when Decision is "replace"; over ExternalizeFieldBytes it travels as null and
// Externalized describes it. Answer is legal only with a block at tool.before:
// the tool does not run and the text stands in for its output.
type InterceptResult struct {
	Decision     InterceptDecision   `json:"decision"`
	Reason       string              `json:"reason,omitempty"`
	Answer       string              `json:"answer,omitempty"`
	Replacement  json.RawMessage     `json:"replacement,omitempty" externalizable:"true"`
	Externalized []ExternalizedField `json:"externalized,omitempty"`
}
