package settings

import (
	"encoding/json"

	"reasonix/internal/contract/tool"
)

// RemedyFor is the fix a refusal points at for a setting and the value that
// would lift it. The operation follows the key's class, so a producer never
// decides it: a Guarded key can only be opened, never proposed.
func RemedyFor(id string, value any) *tool.Remedy {
	e, ok := Lookup(id)
	if !ok {
		return nil
	}
	if e.Class == Guarded {
		return &tool.Remedy{Op: tool.RemedyOpen, Setting: id}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return &tool.Remedy{Op: tool.RemedyPropose, Setting: id, Value: raw}
}

func (c Class) String() string {
	switch c {
	case Preference:
		return "preference"
	case Elevated:
		return "elevated"
	case Guarded:
		return "guarded"
	}
	return ""
}

func (e Effect) String() string {
	switch e {
	case Immediate:
		return "immediate"
	case NextTurn:
		return "next_turn"
	case OnOpen:
		return "on_open"
	case Restart:
		return "restart"
	}
	return ""
}
