package provider

import (
	"strings"
	"testing"
)

func videoMsg(name string) Message {
	return Message{Role: RoleTool, Content: "read_video ok", ToolCallID: name, Name: "read_video",
		Videos: []VideoRef{{Path: "/w/" + name + ".mp4", Name: name + ".mp4", SHA256: "ab", MIME: "video/mp4", Size: 1 << 20, DurationMs: 4000}}}
}

// A video is a request's worth of bytes: it rides the request that follows the
// read, and every later request says it was shown instead of carrying it again.
func TestAVideoRidesOnlyTheRequestBeforeTheModelAnswers(t *testing.T) {
	waiting := []Message{{Role: RoleUser, Content: "look"}, {Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "v1", Name: "read_video"}}}, videoMsg("v1")}
	got := ModelMessages(waiting)
	if len(got[2].Videos) != 1 || strings.Contains(got[2].Content, "was shown") {
		t.Fatalf("the request that carries the read must carry the video: %+v", got[2])
	}

	answered := append(append([]Message(nil), waiting...), Message{Role: RoleAssistant, Content: "a dog"}, Message{Role: RoleUser, Content: "and then?"})
	got = ModelMessages(answered)
	if len(got[2].Videos) != 0 || !strings.Contains(got[2].Content, "v1.mp4") || !strings.Contains(got[2].Content, "is not attached again") {
		t.Fatalf("after the model answered the video must be a note naming the file: %+v", got[2])
	}
	if len(answered[2].Videos) != 1 {
		t.Fatal("projecting must not edit the stored message")
	}
}

func TestOnlyTheLatestUnansweredVideoStaysInTheRequest(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "look at both"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "v1", Name: "read_video"}, {ID: "v2", Name: "read_video"}}},
		videoMsg("v1"), videoMsg("v2"),
	}
	got := ModelMessages(msgs)
	if len(got[2].Videos) != 1 || len(got[3].Videos) != 1 {
		t.Fatalf("both reads were made in one round, so the model has not answered either: %+v %+v", got[2], got[3])
	}
}

// The stored projection compaction rewrites keeps the reference, so a video
// that is still waiting for its answer survives a fold.
func TestProjectionMessagesKeepTheVideoReference(t *testing.T) {
	msgs := []Message{{Role: RoleUser, Content: "look"}, {Role: RoleAssistant, Content: "ok"}, videoMsg("v1")}
	if got := ProjectionMessages(msgs); len(got[2].Videos) != 1 {
		t.Fatalf("a stored projection dropped the reference: %+v", got[2])
	}
}

func TestWithoutVideosSaysWhyTheModelDoesNotSeeIt(t *testing.T) {
	m := WithoutVideos(videoMsg("v1"), VideoNotSentNote)
	if len(m.Videos) != 0 || !strings.Contains(m.Content, "v1.mp4") || !strings.Contains(m.Content, "cannot take video natively") {
		t.Fatalf("a provider that cannot take video must tell the model, not go silent: %+v", m)
	}
}

// Another reason to copy the transcript must not hide that a video was answered:
// a host-authored line earlier in the history stops the scan for reasons early.
func TestAnsweredVideoIsStrippedEvenWhenAnEarlierMessageForcesACopy(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "host line", HostAuthored: true},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "v1", Name: "read_video"}}},
		videoMsg("v1"),
		{Role: RoleAssistant, Content: "a dog"},
	}
	got := ModelMessages(msgs)
	if len(got[2].Videos) != 0 || !strings.Contains(got[2].Content, "is not attached again") {
		t.Fatalf("the answered video survived a copy forced by another message: %+v", got[2])
	}
}
