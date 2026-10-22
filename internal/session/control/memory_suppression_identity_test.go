package control

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/ablation"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/tool"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/state/memory"
	"reasonix/internal/state/sessionstore"
)

const (
	recallIdentitySecret = "SECRET-FACT-BODY-MARKER"
	recallIdentityQuery  = "what is the canonical fast check command"
)

// recallOutcome is one composed-turn recall outcome and the typed identity its
// audit and lastRecall must carry.
type recallOutcome struct {
	name string
	// wantCode cites the typed constant, so a rename is a compile error rather
	// than a silent contract change.
	wantCode memory.SuppressionCode
	// promptCarriesFact is the fixture's positive control: only the carried
	// note may put the fact body in front of the model.
	promptCarriesFact bool
	build             func(t *testing.T) (*Controller, *recallAuditSink)
}

// A composed turn has three recall outcomes — a carried memory note, the
// retrieval switch, or a real search with no match — and a trajectory must tell
// them apart from the audit alone. All three carry a typed identity; none
// carries fact text.
func TestComposeAuditsEveryRecallSuppressionIdentity(t *testing.T) {
	identities := map[memory.SuppressionCode]string{}
	for _, tc := range recallOutcomes() {
		t.Run(tc.name, func(t *testing.T) {
			c, sink := tc.build(t)
			prompt := c.Compose(recallIdentityQuery)
			if got := strings.Contains(prompt, recallIdentitySecret); got != tc.promptCarriesFact {
				t.Fatalf("composed turn carries the fact body = %v, want %v:\n%s", got, tc.promptCarriesFact, prompt)
			}
			if len(sink.audits) != 1 {
				t.Fatalf("audits = %d, want exactly one for this turn", len(sink.audits))
			}
			audit := sink.audits[0]
			if len(audit.Hits) != 0 {
				t.Fatalf("a suppressed recall reported hits: %+v", audit.Hits)
			}
			if strings.Contains(fmt.Sprintf("%+v", audit), recallIdentitySecret) {
				t.Fatalf("audit carries fact content: %+v", audit)
			}
			got := memory.SuppressionCode(audit.SuppressionCode)
			if got != tc.wantCode {
				t.Fatalf("audit suppression code = %q, want %q (Suppressed = %q)", got, tc.wantCode, audit.Suppressed)
			}
			if prev, dup := identities[got]; dup {
				t.Fatalf("this outcome and %q share the identity %q", prev, got)
			}
			identities[got] = tc.name
			last := c.LastMemoryRecall()
			if strings.TrimSpace(last.Query) != recallIdentityQuery || last.SuppressionCode != tc.wantCode {
				t.Fatalf("last recall = query %q code %q, want this turn's %q decision",
					last.Query, last.SuppressionCode, tc.wantCode)
			}
		})
	}
}

// TestSuppressionCodeTellsTheThreeOutcomesApart is the consumer's view: three
// turns, three silent recalls, told apart by the typed field alone. The prose
// sentence is never read.
func TestSuppressionCodeTellsTheThreeOutcomesApart(t *testing.T) {
	seen := map[memory.SuppressionCode]string{}
	for _, tc := range recallOutcomes() {
		c, sink := tc.build(t)
		_ = c.Compose(recallIdentityQuery)
		if len(sink.audits) != 1 {
			t.Fatalf("%s: audits = %d, want exactly one", tc.name, len(sink.audits))
		}
		code := memory.SuppressionCode(sink.audits[0].SuppressionCode)
		if code == "" {
			t.Fatalf("%s: audit carries no typed suppression identity", tc.name)
		}
		if prev, dup := seen[code]; dup {
			t.Fatalf("%s and %s share the typed identity %q", prev, tc.name, code)
		}
		seen[code] = tc.name
	}
	if len(seen) != 3 {
		t.Fatalf("three outcomes produced %d distinct typed codes: %v", len(seen), seen)
	}
}

func recallOutcomes() []recallOutcome {
	return []recallOutcome{
		{
			name: "carried note", wantCode: memory.SuppressCarriedNote, promptCarriesFact: true,
			build: func(t *testing.T) (*Controller, *recallAuditSink) {
				c, sink := recallAuditController(t, nil, Options{})
				if _, err := c.SaveMemory(memory.Memory{
					Name: "alpha", Description: recallIdentitySecret, Type: memory.TypeReference,
					Scope: memory.FactScopeProject, Body: recallIdentitySecret,
				}); err != nil {
					t.Fatalf("SaveMemory: %v", err)
				}
				return c, sink
			},
		},
		{
			name: "retrieval ablated", wantCode: memory.SuppressRetrievalOff,
			build: func(t *testing.T) (*Controller, *recallAuditSink) {
				return recallAuditController(t, []memory.Memory{{
					Name: "canonical-check", Title: "Canonical fast check command",
					Description: "how to run the canonical fast check command",
					Type:        memory.TypeReference, Scope: memory.FactScopeProject, Body: recallIdentitySecret,
				}}, Options{Ablation: ablation.New(ablation.Retrieval)})
			},
		},
		{
			name: "no match", wantCode: memory.SuppressNoMatch,
			build: func(t *testing.T) (*Controller, *recallAuditSink) {
				return recallAuditController(t, []memory.Memory{{
					Name: "budget", Title: "Quarterly budget review",
					Description: "quarterly budget review", Type: memory.TypeReference,
					Scope: memory.FactScopeProject, Body: "The quarterly budget review happens every March.",
				}}, Options{})
			},
		},
	}
}

// recallAuditController wires a real controller over a temp memory store.
// Facts are seeded before the snapshot the controller reads is loaded, so the
// snapshot's prebuilt recall index sees them.
func recallAuditController(t *testing.T, seeded []memory.Memory, opts Options) (*Controller, *recallAuditSink) {
	t.Helper()
	root := testenv.TempDir(t)
	cwd := filepath.Join(root, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	memOpts := memory.Options{CWD: cwd, UserDir: filepath.Join(root, "user")}
	seed := memory.Load(memOpts)
	for _, fact := range seeded {
		if _, err := seed.Store.Save(fact); err != nil {
			t.Fatalf("seed memory %q: %v", fact.Name, err)
		}
	}
	ag := agent.New(nil, tool.NewRegistry(), sessionstore.NewSession("sys"), agent.Options{}, event.Discard)
	sink := &recallAuditSink{Sink: event.Discard}
	opts.Runner, opts.Executor, opts.Sink, opts.Memory = ag, ag, sink, memory.Load(memOpts)
	return New(opts), sink
}
