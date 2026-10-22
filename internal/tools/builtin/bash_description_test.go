package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/tools/jobs"
)

// The model's own name for a background call, and the full command behind it,
// reach the task list; the short preview stays what it was.
func TestBackgroundBashCarriesItsDescriptionAndFullCommand(t *testing.T) {
	m := jobs.NewManager(event.Discard)
	defer m.Close()
	ctx := jobs.WithManager(context.Background(), m)
	command := "echo hello && sleep 0.3 && echo " + strings.Repeat("x", 80)
	args, _ := json.Marshal(map[string]any{"command": command, "run_in_background": true, "description": "  Say hello slowly  "})
	if _, err := (bash{}).Execute(ctx, args); err != nil {
		t.Fatal(err)
	}
	running := m.Running()
	if len(running) != 1 {
		t.Fatalf("running = %+v", running)
	}
	v := running[0]
	if v.Description != "Say hello slowly" || v.Detail != command {
		t.Errorf("view = description %q detail %q", v.Description, v.Detail)
	}
	if v.Label == command || !strings.HasSuffix(v.Label, "…") {
		t.Errorf("the label is no longer the short preview: %q", v.Label)
	}
}

func TestBackgroundBashWithoutADescriptionStillRuns(t *testing.T) {
	m := jobs.NewManager(event.Discard)
	defer m.Close()
	ctx := jobs.WithManager(context.Background(), m)
	if _, err := (bash{}).Execute(ctx, []byte(`{"command":"sleep 0.2","run_in_background":true}`)); err != nil {
		t.Fatal(err)
	}
	running := m.Running()
	if len(running) != 1 || running[0].Description != "" || running[0].Detail != "sleep 0.2" {
		t.Fatalf("running = %+v", running)
	}
}

// Optional, not required: an older prompt that never sends it must keep working.
func TestBashSchemaOffersDescriptionWithoutRequiringIt(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal((bash{}).Schema(), &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties["description"].Type != "string" {
		t.Fatalf("description missing from the schema: %+v", schema.Properties)
	}
	for _, r := range schema.Required {
		if r == "description" {
			t.Fatal("description must stay optional")
		}
	}
}
