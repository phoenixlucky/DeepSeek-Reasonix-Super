package settings

import (
	"reasonix/internal/contract/config"
	"testing"
)

func TestPersonaKeysCannotBeWrittenThroughModelProposals(t *testing.T) {
	for _, id := range []string{"agent.persona.prefix", "agent.persona.suffix", "agent.persona.include_runtime_context"} {
		e, ok := Lookup(id)
		if !ok || e.Class != Guarded || e.Effect != OnOpen {
			t.Fatalf("persona boundary missing from settings catalog: %s %+v", id, e)
		}
		cfg := config.Default()
		if _, _, err := Write(cfg, id, []byte(`"untrusted proposal"`)); CodeOf(err) != CodeGuarded {
			t.Fatalf("model could propose a persona change: %s %v", id, err)
		}
		if !cfg.Agent.Persona.IsZero() {
			t.Fatal("refused proposal changed persona")
		}
	}
}
