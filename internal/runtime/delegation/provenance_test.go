package delegation

import (
	"context"
	"encoding/json"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/runtime/writeclaim"
)

type fakeWebTool struct{ fakeReadFileTool }

func (fakeWebTool) Name() string { return "web_fetch" }

func (fakeWebTool) Provenance(json.RawMessage) tool.Provenance {
	return tool.Provenance{Kind: tool.ProvenanceWeb, Source: "docs.example.com"}
}

func runChild(t *testing.T, reg *tool.Registry, call provider.Chunk) tool.Provenance {
	t.Helper()
	prov := &scriptedProvider{name: "p", turns: [][]provider.Chunk{
		{call, {Type: provider.ChunkDone}},
		{{Type: provider.ChunkText, Text: "report"}, {Type: provider.ChunkDone}},
	}}
	task := NewTaskTool(prov, nil, reg, 20, 0, 0, 0, 0.0, "", "sys", nil, 0, "", "", nil).
		WithTranscripts(mustSubagentStore(t), testenv.TempDir(t), "base", "high").
		WithScheduler(writeclaim.NewSubagentScheduler(4, 4))
	ctx, sink := tool.WithProvenanceSink(agent.WithCallContext(context.Background(), "call-1", event.Discard, nil, false))
	if _, err := task.Execute(ctx, json.RawMessage(`{"prompt":"look it up"}`)); err != nil {
		t.Fatalf("task: %v", err)
	}
	return sink.Origin()
}

// What a child read from the web reaches the parent inside the child's report,
// so the report has to carry that origin or it arrives looking like workspace text.
func TestSubagentReportCarriesTheOriginOfWhatTheChildFetched(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeWebTool{})
	got := runChild(t, reg, toolCallChunk("1", "web_fetch", `{}`))
	if got.Kind != tool.ProvenanceWeb || got.Source != "docs.example.com" {
		t.Fatalf("report origin = %+v, want web:docs.example.com", got)
	}
}

func TestSubagentReportOfWorkspaceReadsIsNotLabelledExternal(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Add(fakeReadFileTool{})
	if got := runChild(t, reg, toolCallChunk("1", "read_file", `{"path":"a.go"}`)); got.External() {
		t.Fatalf("report origin = %+v, want none", got)
	}
}
