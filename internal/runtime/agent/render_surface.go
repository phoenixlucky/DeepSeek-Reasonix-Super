package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"reasonix/internal/base/secrets"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/surfacespec"
	"reasonix/internal/contract/tool"
)

type RenderSurfaceTool struct{}

func NewRenderSurfaceTool() *RenderSurfaceTool { return &RenderSurfaceTool{} }
func (*RenderSurfaceTool) Name() string        { return surfacespec.ToolName }
func (*RenderSurfaceTool) Description() string {
	return "Show a native structured form or choice cards for information only the user can supply. Fields accept string, number, bool, choice or multichoice values keyed by stable ids. Display-only trees return a summary. Interactive trees wait for the user through ask. Never use surfaces to approve tools or grant permissions. No HTML, CSS, URLs or executable components."
}
func (*RenderSurfaceTool) ReadOnly() bool        { return true }
func (*RenderSurfaceTool) Reach() tool.Reach     { return tool.ReachHostControl }
func (*RenderSurfaceTool) DecisionBarrier() bool { return true }
func (*RenderSurfaceTool) SnipHint() tool.SnipHint {
	return tool.SnipHint{Head: 4, Tail: 2, HeadChars: 3000, TailChars: 1000}
}
func (*RenderSurfaceTool) Schema() json.RawMessage { return surfaceSchema }

func (*RenderSurfaceTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	s, err := surfacespec.Parse(args)
	if err != nil {
		return "", err
	}
	s.Redact(secrets.RedactCredentials)
	if err := s.Validate(); err != nil {
		return "", err
	}
	canonical, err := s.Canonical()
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("surface-%x", sha256.Sum256(canonical))[:24]
	if len(s.Fields()) == 0 {
		return "surface_id: " + id + "\nDisplayed: " + s.Title, nil
	}
	callID, _, asker, ok := CallContext(ctx)
	if !ok || asker == nil {
		return "unresolved: this run has no interactive user. Do not supply their answers or choose on their behalf.", nil
	}
	answers, err := asker.Ask(ctx, []event.AskQuestion{{ID: "surface", Header: s.Title, Prompt: s.Title, Surface: s, CallID: callID}})
	if timedOut, ok := errors.AsType[*AskTimedOutError](err); ok {
		return timedOut.result(), nil
	}
	if err != nil {
		return "", err
	}
	if len(answers) == 0 {
		return "The user dismissed the surface without answering. Stop and wait for their next message.", nil
	}
	if len(answers) != 1 || answers[0].QuestionID != "surface" || len(answers[0].Selected) != 0 {
		return "", surfacespec.ErrValueInvalid
	}
	values, err := s.ParseAnswers(answers[0].Value)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(struct {
		SurfaceID string                     `json:"surface_id"`
		Values    map[string]json.RawMessage `json:"values"`
	}{id, values})
	return string(result), err
}
