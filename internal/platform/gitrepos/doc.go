// Package gitrepos finds the repositories a workspace holds when the workspace
// folder is not itself one: a folder of projects whose children each carry
// their own .git. The answer is structural: a directory counts when git resolves
// it as the top of its own work tree, never because of what it is called, and
// the search is bounded in depth, in directories read and in repositories kept,
// so a large folder cannot turn opening a panel into a disk walk.
package gitrepos
