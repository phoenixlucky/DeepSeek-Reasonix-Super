package control

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/state/checkpoint"
	"reasonix/internal/state/sessionstore"
)

func TestBranchAtKeepsTheSourceAndCutsAfterTheReply(t *testing.T) {
	c, _, _ := runTwoTurns(t)
	defer c.Close()
	parent := c.SessionPath()
	before := c.History()
	cp := c.Checkpoints()[0]
	path, err := c.BranchAt(cp.Turn, "")
	if err != nil {
		t.Fatal(err)
	}
	child, err := sessionstore.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	msgs := child.Snapshot()
	if msgs[len(msgs)-1].Content != "first answer" || len(msgs) >= len(before) {
		t.Fatalf("fork contains the wrong prefix: %+v", msgs)
	}
	if c.SessionPath() != parent || !reflect.DeepEqual(before, c.History()) {
		t.Fatal("fork changed the source conversation")
	}
	meta, _, err := sessionstore.LoadBranchMeta(path)
	if err != nil || meta.ParentID != sessionstore.BranchID(parent) {
		t.Fatalf("fork parent: %+v, %v", meta, err)
	}

}

func TestBranchAtRefusesRunningConversation(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	defer c.Close()
	checkpoints := c.Checkpoints()
	turn := checkpoints[len(checkpoints)-1].Turn + 1
	release := make(chan struct{})
	c.runner = blockingRunner{session: ag.Session(), release: release}
	done := make(chan error, 1)
	go func() { done <- c.RunTurn(context.Background(), "running") }()
	waitForRunning(t, c)
	_, err := c.BranchAt(turn, "")
	close(release)
	<-done
	if !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("fork during a running turn: %v", err)
	}
}

func TestBranchAtKeepsToolsAndReopensConversationCheckpoints(t *testing.T) {
	root := testenv.TempDir(t)
	dir := filepath.Join(root, "sessions")
	ag := agent.New(&scriptedTurns{turns: [][]provider.Chunk{textTurn("first answer"), textTurn("second answer")}}, tool.NewRegistry(), sessionstore.NewSession("sys"), agent.Options{}, event.Discard)
	c := New(Options{Runner: ag, Executor: ag, SessionDir: dir, SessionPath: filepath.Join(dir, "parent.jsonl"), WorkspaceRoot: root, Sink: event.Discard})
	defer c.Close()
	for _, input := range []string{"first prompt", "second prompt"} {
		if err := c.RunTurn(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	sourceStore := c.checkpoints.storeRef()
	sourceStore.CaptureBefore(file, checkpoint.CaptureBeforeOpts{Source: checkpoint.CaptureBeforeMutation})
	if err := os.WriteFile(file, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}
	sourceStore.CaptureAfter(file, checkpoint.CaptureAfterOpts{Seq: 1, Source: checkpoint.CaptureAfterMutation})
	ag.Session().Add(provider.Message{Role: provider.RoleAssistant, ToolCalls: []provider.ToolCall{{ID: "edit-1", Name: "edit_file", Arguments: `{"path":"a.txt"}`}}})
	ag.Session().Add(provider.Message{Role: provider.RoleTool, ToolCallID: "edit-1", Name: "edit_file", Content: "changed a.txt from before to after"})
	ag.Session().Add(provider.Message{Role: provider.RoleAssistant, Content: "file edited"})
	history, checkpoints := c.History(), c.Checkpoints()
	target := checkpoints[len(checkpoints)-1]
	if len(target.Paths) != 1 {
		t.Fatalf("source checkpoint has no file snapshot: %+v", target)
	}
	for _, input := range []string{target.Prompt, "rewritten question"} {
		path, err := c.BranchAt(target.Turn, "")
		if err != nil {
			t.Fatal(err)
		}
		session, err := sessionstore.LoadSession(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(history, session.Snapshot()) {
			t.Fatal("fork dropped or changed tool history")
		}
		executor := agent.New(&scriptedTurns{turns: [][]provider.Chunk{textTurn("branch answer")}}, tool.NewRegistry(), session, agent.Options{}, event.Discard)
		child := New(Options{Runner: executor, Executor: executor, SessionDir: filepath.Dir(path), SessionPath: path, WorkspaceRoot: root})
		copied := child.Checkpoints()
		if len(copied) != len(checkpoints) || copied[1].Turn != target.Turn || copied[1].MsgIndex != target.MsgIndex || len(copied[1].Paths) != 0 {
			t.Fatalf("reopened fork checkpoints: %+v", copied)
		}
		nested, err := child.BranchAt(target.Turn, "nested branch")
		if err != nil {
			t.Fatal(err)
		}
		if savedMessages(t, nested) != len(history) {
			t.Fatal("reopened branch cut at the wrong turn")
		}
		if len(checkpoint.New(ckptDir(nested), root).List()) != len(checkpoints) {
			t.Fatal("nested branch lost copied checkpoints")
		}
		plan, err := child.PrepareRewind(target.Turn, RewindConversation)
		if err != nil || !plan.CanConversation {
			t.Fatalf("prepare conversation rewind: %+v, %v", plan, err)
		}
		result, err := child.CommitRewind(plan.PlanID)
		if err != nil || !result.ConversationOK {
			t.Fatalf("rewind fork: %+v, %v", result, err)
		}
		if err := child.RunTurn(context.Background(), input); err != nil {
			t.Fatal(err)
		}
		messages := child.History()
		if messages[target.MsgIndex].RawContent != input || messages[len(messages)-1].Content != "branch answer" {
			t.Fatalf("wrong regenerated or rewritten turn: %+v", messages)
		}
		child.Close()
	}
	if !reflect.DeepEqual(history, c.History()) || !reflect.DeepEqual(checkpoints, c.Checkpoints()) || string(mustReadFile(t, file)) != "after" {
		t.Fatal("fork or conversation rewind changed the source or workspace file")
	}
}

func TestBranchAtClosedTurnCopiesCheckpointsAndSettings(t *testing.T) {
	c, ag, _ := runTwoTurns(t)
	defer c.Close()
	c.SetAgentPreset("delivery")
	c.SetPlanMode(true)
	turn := orderedTurns(t, c)[0]
	startLaterTurn(t, c, ag)
	path, err := c.BranchAt(turn, "closed turn")
	if err != nil {
		t.Fatal(err)
	}
	copied := checkpoint.New(ckptDir(path), c.WorkspaceRoot()).List()
	if len(copied) != 1 || !copied[0].ForkCopied || copied[0].Turn != turn || len(copied[0].Paths) != 0 {
		t.Fatalf("closed-turn branch lost conversation checkpoints: %+v", copied)
	}
	meta, _, err := sessionstore.LoadBranchMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Model != c.ModelRef() || meta.AgentPreset != "delivery" || meta.Mode != "plan" {
		t.Fatalf("closed-turn branch lost settings: %+v", meta)
	}
}
