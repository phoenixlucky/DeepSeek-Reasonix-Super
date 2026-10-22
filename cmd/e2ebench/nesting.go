package main

import "reasonix/internal/contract/tool"

// callNesting tells the model's own calls from calls made on its behalf: a
// subagent's (stamped with ParentID) and those a running tool makes through
// the agent's invoker (run_script's), which carry an id derived from their
// caller's and arrive inside its dispatch→result window. Either kind ran
// inside one top-level call, so it bought no model request of its own.
type callNesting struct {
	open map[string]bool // top-level calls dispatched and not yet resulted
}

// child reports whether a tool event belongs to a call made on the model's
// behalf, and tracks which top-level calls are still running.
func (n *callNesting) child(kind, id, parentID string) bool {
	if parentID != "" {
		return true
	}
	for caller := range n.open {
		if tool.IsNestedCallOf(id, caller) {
			return true
		}
	}
	if id == "" {
		return false
	}
	switch kind {
	case "tool_dispatch":
		if n.open == nil {
			n.open = map[string]bool{}
		}
		n.open[id] = true
	case "tool_result":
		delete(n.open, id)
	}
	return false
}
