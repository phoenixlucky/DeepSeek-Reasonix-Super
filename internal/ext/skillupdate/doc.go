// Package skillupdate brings an installed copy of a skill up to date with the
// folder it was installed from. It is a host operation a person starts from the
// window, never a tool the model can call: an installed skill is instructions
// and scripts the agent later follows, so changing it is the user's decision.
//
// Plan reads both sides and describes what an update would change; Apply
// repeats that reading and refuses unless it still matches the plan the person
// saw (the plan id digests the whole plan, the installed copy's current digest
// and the recorded source). The new content is written beside the old and
// swapped in by rename, so a failure part way leaves the old copy as it was.
package skillupdate
