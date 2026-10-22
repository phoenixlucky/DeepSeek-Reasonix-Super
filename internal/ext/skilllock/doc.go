// Package skilllock remembers where an installed skill came from, so a copy
// installed from a source folder can be told apart from that folder.
//
// The record lives in skills.lock beside the skills folder, inside the Reasonix
// home the host refuses to grant for writing: the agent can neither edit it nor
// the installed copy it describes. Each entry carries the content digest of what
// was installed. Whether a skill is in step with its source is a comparison of
// three digests (installed now, installed then, source now); a missing or
// unreadable record, an unreadable source or a remote one reads as unknown, never
// as in step.
package skilllock
