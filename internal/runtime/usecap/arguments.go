package usecap

import (
	"encoding/json"
	"strings"

	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
	"reasonix/internal/runtime/capability"
)

// PrepareArguments reads only already-registered targets; discovering a schema
// must not start an MCP server or spend permission before execution.
func (t *UseCapabilityTool) PrepareArguments(args json.RawMessage) (json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(args, &fields) != nil || t.registry == nil {
		return args, false
	}
	var action, id string
	if json.Unmarshal(fields["action"], &action) != nil || strings.ToLower(strings.TrimSpace(action)) != "call" ||
		json.Unmarshal(fields["capability_id"], &id) != nil {
		return args, false
	}
	id = strings.TrimSpace(id)
	name, builtin := strings.CutPrefix(id, "tool:")
	if !builtin {
		name = ""
		if entry, ok := t.currentCatalog().Lookup(id); ok && entry.Kind == capability.KindTool {
			name = entry.ToolName
		}
		if server, raw, err := parseMCPCapabilityID(id); name == "" && err == nil {
			name = plugin.ModelToolName(server, raw)
		}
	}
	if name == "" || name == "use_capability" {
		return args, false
	}
	target, ok := t.registry.Get(strings.TrimSpace(name))
	if !ok {
		return args, false
	}
	effective, changed := tool.UnwrapArguments(target.Schema(), fields["arguments"])
	if !changed {
		return args, false
	}
	fields["arguments"] = effective
	out, err := json.Marshal(fields)
	if err != nil {
		return args, false
	}
	return out, true
}
