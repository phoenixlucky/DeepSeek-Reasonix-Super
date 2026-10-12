package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
)

type surfaceAsker struct{ questions []event.AskQuestion }

func (a *surfaceAsker) Ask(_ context.Context, qs []event.AskQuestion) ([]event.AskAnswer, error) {
	a.questions = qs
	return []event.AskAnswer{{QuestionID: "surface", Value: json.RawMessage(`{"count":2}`)}}, nil
}

func TestRenderSurfaceUsesTypedAsk(t *testing.T) {
	a := &surfaceAsker{}
	ctx := WithCallContext(context.Background(), "call-1", event.Discard, a, false)
	result, err := NewRenderSurfaceTool().Execute(ctx, []byte(`{"spec_version":1,"title":"Settings","body":[{"kind":"field","key":"count","label":"Count","field":{"type":"number","required":true}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.questions) != 1 || a.questions[0].Surface == nil {
		t.Fatal(a.questions)
	}
	if !strings.Contains(result, `"count":2`) {
		t.Fatal(result)
	}
	result, err = NewRenderSurfaceTool().Execute(context.Background(), []byte(`{"spec_version":1,"body":[{"kind":"field","key":"name","label":"Name","field":{"type":"string"}}]}`))
	if err != nil || !strings.Contains(result, "unresolved") {
		t.Fatalf("headless: %s %v", result, err)
	}
}
