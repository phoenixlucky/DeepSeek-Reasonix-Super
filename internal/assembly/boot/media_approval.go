package boot

import "reasonix/internal/safety/permission"

// askWithUnsandboxedMedia is the ask list the permission policy is built from.
// read_video decodes media with a program that parses untrusted bytes; where an
// OS sandbox wraps that program it is bounded, and where none does (Windows, or
// the sandbox turned off) the first use is sent for approval. A rule the user
// wrote about read_video, in any list, is theirs and leaves this alone.
func askWithUnsandboxedMedia(allow, ask, deny []string, sandboxed bool) []string {
	if sandboxed {
		return ask
	}
	for _, list := range [][]string{allow, ask, deny} {
		for _, rule := range list {
			if permission.RuleMatchesString(rule, "read_video", "") {
				return ask
			}
		}
	}
	return append(append([]string(nil), ask...), "read_video")
}
