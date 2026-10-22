package websearch

import (
	"context"
	"errors"
	"fmt"
)

// Hit is one normalised search result.
type Hit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet,omitempty"`
}

// Backend answers a search. Paid backends are reached only after the free ones
// failed and only with the user's consent.
type Backend interface {
	Name() string
	Paid() bool
	Search(ctx context.Context, query string) ([]Hit, error)
}

// FailKind is why a backend did not answer. The producer knows; the chain and
// the notices only carry it.
type FailKind string

const (
	FailTransport FailKind = "transport"
	FailStatus    FailKind = "status"
	FailDecode    FailKind = "decode"
	FailEmpty     FailKind = "empty"
)

// BackendError is a backend's failure with its structural cause. Status is set
// for FailStatus.
type BackendError struct {
	Backend string
	Kind    FailKind
	Status  int
	// Code is the identity the backend gives its own failure, when it has one
	// (the DeepSeek API's balance and key problems); empty otherwise.
	Code string
	Err  error
}

func (e *BackendError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s backend: %s (HTTP %d)", e.Backend, e.Kind, e.Status)
	}
	return fmt.Sprintf("%s backend: %s", e.Backend, e.Kind)
}

// identity is what the model is told: the backend's own code, else the kind.
func (e *BackendError) identity() string {
	if e.Code != "" {
		return e.Code
	}
	return string(e.Kind)
}

func (e *BackendError) Unwrap() error { return e.Err }

func failure(err error) *BackendError {
	var be *BackendError
	if errors.As(err, &be) {
		return be
	}
	return &BackendError{Kind: FailTransport, Err: err}
}
