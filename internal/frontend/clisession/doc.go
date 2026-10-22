// Package clisession is what a process that runs one agent session without a
// terminal UI shares with the command line: which directory its sessions live
// in, and binding a run to its session file under the write lease. The scheduled
// child and the CLI both start from here, so neither carries the other.
package clisession
