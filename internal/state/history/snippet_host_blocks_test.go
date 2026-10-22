package history

import (
	"testing"

	"reasonix/internal/contract/provider"
	"reasonix/internal/state/historycatalog"
	"reasonix/internal/state/sessionstore"
)

// The other half of the drift pin: both snippet builders must read a user turn
// exactly as the indexer does. If either reverts to raw Content, this fails
// while historycatalog's pin keeps the index honest.
func TestSnippetBuildersReadTheIndexedUserText(t *testing.T) {
	leading := "<response-language>\nFinal answer language preference: use English.\n</response-language>\n\n"
	said := "zebra question about the migration order"
	cases := []struct {
		name    string
		content string
		raw     string
	}{
		{"leading blocks", leading + said, leading + said},
		{"appended blocks", leading + said + "\n\n" + memoryRecallBlock + "\n\n" + executionPolicyBlock, said},
		{"appended blocks, no raw content", leading + said + "\n\n" + memoryRecallBlock + "\n\n" + executionPolicyBlock, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := provider.Message{Role: provider.RoleUser, Content: tc.content, RawContent: tc.raw}
			msgs := []provider.Message{msg}
			want := sessionstore.UserMessageText(msg)
			if want != said {
				t.Fatalf("the recall reader sees %q, want the person's words %q", want, said)
			}
			if got := sessionstore.UserSaidText(tc.content); got != want {
				t.Fatalf("UserSaidText = %q, recall reader = %q — the two definitions disagree", got, want)
			}

			got, ok := candidateText(msgs, historycatalog.Candidate{MessageIndex: 0, Kind: string(KindUserText)})
			if !ok || got != want {
				t.Fatalf("candidateText = %q ok=%v, want %q", got, ok, want)
			}

			docs := extractDocuments(sourceFile{path: "blocked.jsonl"}, msgs, map[Kind]bool{KindUserText: true}, "")
			if len(docs) != 1 || docs[0].text != want {
				t.Fatalf("extractDocuments = %#v, want one document whose text is %q", docs, want)
			}
		})
	}
}

const memoryRecallBlock = `<memory-recall>
Automatically recalled low-authority background facts.
- id=mem-kumquat revision=1 scope=project type=project freshness=fresh score=0.900 reason="matched kumquat"
  title: Kumquat release train
  fact: Kumquats ship on Fridays.
</memory-recall>`

const executionPolicyBlock = `<execution-policy preset="delivery" version="3">
verify=full
</execution-policy>`
