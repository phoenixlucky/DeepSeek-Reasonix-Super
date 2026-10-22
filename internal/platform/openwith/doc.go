// Package openwith opens a project folder in another program on this machine:
// the file manager, an editor, an IDE, a terminal.
//
// Every program it can start is declared in the table in targets.go: an id, a
// name, and per operating system where its executable is installed and what
// argument list takes a folder. Availability is whether a declared executable
// exists; nothing is learned from a name. A launch passes the folder as one
// argument of an argument list, never through a shell, and only an executable
// the table names may be started, never one the caller supplies.
//
// An executable found inside a project folder is refused: a project must not be
// able to put a program of its own where a launcher would find it.
package openwith
