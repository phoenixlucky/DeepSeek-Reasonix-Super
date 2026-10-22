package plugin

import (
	"context"
	"testing"
)

func TestNewTransportFailureIsANilInterface(t *testing.T) {
	for _, spec := range []Spec{
		{Name: "a", Type: "stdio", Command: "/definitely/not/a/real/command"},
		{Name: "b", Type: "http"},
		{Name: "c", Type: "sse"},
	} {
		tr, err := newTransport(context.Background(), spec)
		if err == nil {
			t.Fatalf("%s: expected a launch failure", spec.Type)
		}
		if tr != nil {
			t.Fatalf("%s: a failed transport must be a nil interface, got %T", spec.Type, tr)
		}
	}
}
